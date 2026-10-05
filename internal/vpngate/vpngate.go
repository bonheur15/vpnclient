// Package vpngate fetches and parses the public VPN Gate server list.
package vpngate

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const apiURL = "https://www.vpngate.net/api/iphone/"

// Server is one entry from the VPN Gate list.
type Server struct {
	ID           string `json:"id"`
	HostName     string `json:"hostname"`
	IP           string `json:"ip"`
	Score        int64  `json:"score"`
	PingMs       int    `json:"pingMs"`
	SpeedBps     int64  `json:"speedBps"`
	CountryLong  string `json:"country"`
	CountryShort string `json:"countryCode"`
	Sessions     int64  `json:"sessions"`
	UptimeMs     int64  `json:"uptimeMs"`
	TotalUsers   int64  `json:"totalUsers"`
	Operator     string `json:"operator"`
	Proto        string `json:"proto"`
	Port         int    `json:"port"`
	ConfigB64    string `json:"-"`
}

// Key is the stable identity used for persisted per-server stats.
func (s *Server) Key() string {
	return fmt.Sprintf("%s:%s:%d", s.IP, s.Proto, s.Port)
}

// Config returns the decoded OpenVPN config text.
func (s *Server) Config() (string, error) {
	b, err := base64.StdEncoding.DecodeString(s.ConfigB64)
	if err != nil {
		return "", fmt.Errorf("decode openvpn config: %w", err)
	}
	return string(b), nil
}

// Fetch downloads the current server list. cacheDir, if non-empty, is used to
// persist the raw CSV so the app can start offline; on download failure the
// cache is used as a fallback regardless of age.
func Fetch(ctx context.Context, cacheDir string) ([]Server, error) {
	raw, err := download(ctx)
	if err != nil {
		if cached, cerr := os.ReadFile(cachePath(cacheDir)); cerr == nil {
			servers, perr := Parse(cached)
			if perr == nil && len(servers) > 0 {
				return servers, nil
			}
		}
		return nil, err
	}
	servers, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	if cacheDir != "" && len(servers) > 0 {
		_ = os.WriteFile(cachePath(cacheDir), raw, 0o644)
	}
	return servers, nil
}

// LoadCache parses the on-disk cache without touching the network.
func LoadCache(cacheDir string) ([]Server, time.Time, error) {
	p := cachePath(cacheDir)
	fi, err := os.Stat(p)
	if err != nil {
		return nil, time.Time{}, err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, time.Time{}, err
	}
	servers, err := Parse(raw)
	return servers, fi.ModTime(), err
}

func cachePath(dir string) string { return filepath.Join(dir, "servers_cache.csv") }

func download(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "vpngate-client/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch vpngate list: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch vpngate list: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// Parse decodes the VPN Gate CSV format. Lines starting with '*' are section
// markers and the header line starts with '#'.
func Parse(raw []byte) ([]Server, error) {
	r := csv.NewReader(strings.NewReader(string(raw)))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var out []Server
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		if len(rec) < 15 || strings.HasPrefix(rec[0], "*") || strings.HasPrefix(rec[0], "#") {
			continue
		}
		s := Server{
			HostName:     rec[0],
			IP:           rec[1],
			Score:        atoi64(rec[2]),
			PingMs:       int(atoi64(rec[3])),
			SpeedBps:     atoi64(rec[4]),
			CountryLong:  rec[5],
			CountryShort: strings.ToUpper(rec[6]),
			Sessions:     atoi64(rec[7]),
			UptimeMs:     atoi64(rec[8]),
			TotalUsers:   atoi64(rec[9]),
			Operator:     rec[12],
			ConfigB64:    rec[14],
		}
		if s.IP == "" || s.ConfigB64 == "" {
			continue
		}
		s.Proto, s.Port = parseEndpoint(s.ConfigB64)
		if s.Port == 0 {
			continue
		}
		h := sha1.Sum([]byte(s.Key()))
		s.ID = hex.EncodeToString(h[:8])
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("vpngate list: no servers parsed")
	}
	return out, nil
}

// parseEndpoint extracts "proto" and the port of the first "remote" line from
// a base64 OpenVPN config.
func parseEndpoint(b64 string) (proto string, port int) {
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", 0
	}
	proto = "udp"
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "proto "):
			f := strings.Fields(line)
			if len(f) >= 2 {
				proto = strings.TrimSuffix(f[1], "-client")
			}
		case strings.HasPrefix(line, "remote ") && port == 0:
			f := strings.Fields(line)
			if len(f) >= 3 {
				port, _ = strconv.Atoi(f[2])
			}
		}
	}
	return proto, port
}

func atoi64(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v
}
