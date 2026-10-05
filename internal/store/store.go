// Package store persists per-server connection stats and app settings as JSON.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ServerStats is the locally learned history for one server (keyed by ip:proto:port).
type ServerStats struct {
	Successes        int       `json:"successes"`
	Failures         int       `json:"failures"`
	ConsecutiveFails int       `json:"consecutiveFails"`
	LastSuccess      time.Time `json:"lastSuccess,omitzero"`
	LastFailure      time.Time `json:"lastFailure,omitzero"`
	AvgConnectMs     int64     `json:"avgConnectMs"`
	LastLatencyMs    int64     `json:"lastLatencyMs"` // 0 = untested, -1 = unreachable
	Favorite         bool      `json:"favorite"`
}

// Settings are the user-tunable knobs, editable from the UI.
type Settings struct {
	AutoConnect       bool     `json:"autoConnect"`       // start connecting on app launch
	AutoReconnect     bool     `json:"autoReconnect"`     // reconnect when a live connection drops
	Countries         []string `json:"countries"`         // preferred country codes for auto mode (empty = any)
	ConnectTimeoutSec int      `json:"connectTimeoutSec"` // max time to reach CONNECTED per attempt
	DeadThreshold     int      `json:"deadThreshold"`     // consecutive failures before a server is "dead"
	DeadCooldownMin   int      `json:"deadCooldownMin"`   // minutes a dead server is skipped in auto mode
	ManageDNS         bool     `json:"manageDns"`         // hook resolv.conf update scripts if present
	PrecheckTCP       bool     `json:"precheckTcp"`       // quick TCP reachability probe before full connect
	MaxAutoCandidates int      `json:"maxAutoCandidates"` // cap of servers tried per auto round
}

func DefaultSettings() Settings {
	return Settings{
		AutoConnect:       false,
		AutoReconnect:     true,
		Countries:         []string{},
		ConnectTimeoutSec: 25,
		DeadThreshold:     3,
		DeadCooldownMin:   360,
		ManageDNS:         true,
		PrecheckTCP:       true,
		MaxAutoCandidates: 30,
	}
}

type data struct {
	Stats    map[string]*ServerStats `json:"stats"`
	Settings Settings                `json:"settings"`
}

// Store is a mutex-guarded JSON file. Every mutation saves synchronously;
// write volume is tiny (a few writes per connection attempt).
type Store struct {
	mu   sync.Mutex
	path string
	d    data
}

func Load(path string) (*Store, error) {
	s := &Store{path: path, d: data{Stats: map[string]*ServerStats{}, Settings: DefaultSettings()}}
	raw, err := os.ReadFile(path)
	if err == nil {
		if jerr := json.Unmarshal(raw, &s.d); jerr != nil {
			// Corrupt state file: start fresh rather than failing to boot.
			s.d = data{Stats: map[string]*ServerStats{}, Settings: DefaultSettings()}
		}
		if s.d.Stats == nil {
			s.d.Stats = map[string]*ServerStats{}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func (s *Store) save() {
	raw, err := json.MarshalIndent(&s.d, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err == nil {
		_ = os.Rename(tmp, s.path)
	}
}

// Stats returns a copy of the stats for key (zero value if unseen).
func (s *Store) Stats(key string) ServerStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.d.Stats[key]; ok {
		return *st
	}
	return ServerStats{}
}

func (s *Store) mutate(key string, fn func(*ServerStats)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.d.Stats[key]
	if !ok {
		st = &ServerStats{}
		s.d.Stats[key] = st
	}
	fn(st)
	s.save()
}

func (s *Store) RecordSuccess(key string, connectMs int64) {
	s.mutate(key, func(st *ServerStats) {
		st.Successes++
		st.ConsecutiveFails = 0
		st.LastSuccess = time.Now()
		if st.AvgConnectMs == 0 {
			st.AvgConnectMs = connectMs
		} else {
			st.AvgConnectMs = (st.AvgConnectMs*3 + connectMs) / 4
		}
	})
}

func (s *Store) RecordFailure(key string) {
	s.mutate(key, func(st *ServerStats) {
		st.Failures++
		st.ConsecutiveFails++
		st.LastFailure = time.Now()
	})
}

func (s *Store) RecordLatency(key string, ms int64) {
	s.mutate(key, func(st *ServerStats) { st.LastLatencyMs = ms })
}

func (s *Store) ToggleFavorite(key string) bool {
	var v bool
	s.mutate(key, func(st *ServerStats) {
		st.Favorite = !st.Favorite
		v = st.Favorite
	})
	return v
}

func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.d.Settings
}

func (s *Store) SetSettings(cfg Settings) {
	// Clamp to sane ranges so a bad PUT can't wedge the engine.
	if cfg.ConnectTimeoutSec < 10 {
		cfg.ConnectTimeoutSec = 10
	}
	if cfg.ConnectTimeoutSec > 120 {
		cfg.ConnectTimeoutSec = 120
	}
	if cfg.DeadThreshold < 1 {
		cfg.DeadThreshold = 1
	}
	if cfg.DeadCooldownMin < 5 {
		cfg.DeadCooldownMin = 5
	}
	if cfg.MaxAutoCandidates < 3 {
		cfg.MaxAutoCandidates = 3
	}
	if cfg.Countries == nil {
		cfg.Countries = []string{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.d.Settings = cfg
	s.save()
}
