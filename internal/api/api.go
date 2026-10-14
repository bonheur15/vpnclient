// Package api exposes the engine over REST + Server-Sent Events and serves
// the embedded web UI.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	"vpngate-client/internal/engine"
	"vpngate-client/internal/setup"
	"vpngate-client/internal/store"
	"vpngate-client/internal/vpn"
)

// App holds process-level details and hooks owned by main.
type App struct {
	Version string
	DataDir string
	Quit    func() // shut the whole app down
	Show    func() // bring the UI window/browser to the front
}

type Server struct {
	eng   *engine.Engine
	st    *store.Store
	setup *setup.Manager
	app   App
	web   fs.FS
	ctx   context.Context
}

// New builds the HTTP handler. ctx outlives individual requests and bounds
// background work (like installs) that requests kick off.
func New(ctx context.Context, eng *engine.Engine, st *store.Store, sm *setup.Manager, app App, web fs.FS) http.Handler {
	s := &Server{eng: eng, st: st, setup: sm, app: app, web: web, ctx: ctx}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/status", s.getStatus)
	mux.HandleFunc("GET /api/servers", s.getServers)
	mux.HandleFunc("POST /api/servers/refresh", s.refreshServers)
	mux.HandleFunc("GET /api/recommendations", s.getRecommendations)
	mux.HandleFunc("POST /api/connect", s.postConnect)
	mux.HandleFunc("POST /api/auto", s.postAuto)
	mux.HandleFunc("POST /api/disconnect", s.postDisconnect)
	mux.HandleFunc("POST /api/ping", s.postPing)
	mux.HandleFunc("POST /api/favorite", s.postFavorite)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("GET /api/logs", s.getLogs)
	mux.HandleFunc("GET /api/preflight", s.getPreflight)
	mux.HandleFunc("GET /api/setup", s.getSetup)
	mux.HandleFunc("POST /api/setup/fix", s.postSetupFix)
	mux.HandleFunc("GET /api/info", s.getInfo)
	mux.HandleFunc("POST /api/quit", s.postQuit)
	mux.HandleFunc("POST /api/show", s.postShow)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("/", s.static)
	return sameOrigin(mux)
}

// sameOrigin keeps other websites away from the API, which has no auth (it
// listens on localhost):
//   - Host must be an IP literal or "localhost". This defeats DNS rebinding,
//     where an attacker's domain re-resolves to 127.0.0.1 to read the API.
//   - State-changing requests must not come from a foreign origin, so a page
//     open in the browser can't POST to trigger installs, connects or quit.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !trustedHost(r.Host) {
			writeErr(w, http.StatusForbidden, errors.New("unexpected Host header"))
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin := r.Header.Get("Origin"); origin != "" && !trustedOrigin(origin, r.Host) {
				writeErr(w, http.StatusForbidden, errors.New("cross-origin request refused"))
				return
			}
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				writeErr(w, http.StatusForbidden, errors.New("cross-site request refused"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func trustedHost(hostport string) bool {
	h := hostport
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		h = host
	}
	h = strings.Trim(h, "[]")
	return h == "localhost" || net.ParseIP(h) != nil
}

// trustedOrigin accepts our own origin plus any loopback origin (the Vite
// dev server proxies the API from another localhost port).
func trustedOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Host == host {
		return true
	}
	h := u.Hostname()
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.eng.Status())
}

func (s *Server) getServers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.eng.Servers())
}

func (s *Server) refreshServers(w http.ResponseWriter, r *http.Request) {
	if err := s.eng.RefreshServers(r.Context(), true); err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) getRecommendations(w http.ResponseWriter, r *http.Request) {
	countries := splitCSV(r.URL.Query().Get("countries"))
	recs := s.eng.Recommendations(8, countries)
	if recs == nil {
		recs = []engine.ServerView{}
	}
	writeJSON(w, 200, recs)
}

func (s *Server) postConnect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		writeErr(w, 400, fmt.Errorf("body must be {\"id\": \"...\"}"))
		return
	}
	if err := s.eng.Connect(body.ID); err != nil {
		writeErr(w, 404, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) postAuto(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Countries []string `json:"countries"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := s.eng.AutoConnect(body.Countries); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) postDisconnect(w http.ResponseWriter, r *http.Request) {
	s.eng.Disconnect()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) postPing(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	n := s.eng.PingSweep(r.Context(), body.IDs)
	writeJSON(w, 200, map[string]int{"probed": n})
}

func (s *Server) postFavorite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		writeErr(w, 400, fmt.Errorf("body must be {\"id\": \"...\"}"))
		return
	}
	if err := s.eng.ToggleFavorite(body.ID); err != nil {
		writeErr(w, 404, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.st.Settings())
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	cfg := s.st.Settings() // fields omitted by the client keep their values
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, s.eng.SaveSettings(cfg))
}

func (s *Server) getLogs(w http.ResponseWriter, r *http.Request) {
	logs := s.eng.Logs()
	if logs == nil {
		logs = []engine.LogLine{}
	}
	writeJSON(w, 200, logs)
}

// getPreflight tells the UI whether OpenVPN can actually be launched and
// whether other VPN software would sabotage the tunnel.
func (s *Server) getPreflight(w http.ResponseWriter, r *http.Request) {
	warnings := vpn.DetectConflicts()
	if warnings == nil {
		warnings = []string{}
	}
	res := map[string]any{"ok": true, "warnings": warnings}
	if st := s.setup.State(); !st.Ready() {
		res["ok"] = false
		res["error"] = st.Message
	}
	writeJSON(w, 200, res)
}

func (s *Server) getSetup(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.setup.State())
}

// postSetupFix runs the one-click fix (install / grant access). Progress is
// streamed as "setup" events; the response returns immediately.
func (s *Server) postSetupFix(w http.ResponseWriter, r *http.Request) {
	if err := s.setup.Fix(s.ctx); err != nil {
		writeErr(w, 409, err)
		return
	}
	writeJSON(w, 202, map[string]bool{"ok": true})
}

func (s *Server) getInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"version":   s.app.Version,
		"os":        runtime.GOOS,
		"arch":      runtime.GOARCH,
		"dataDir":   s.app.DataDir,
		"minRotate": store.MinRotateIntervalMin,
		"canQuit":   s.app.Quit != nil,
	})
}

func (s *Server) postQuit(w http.ResponseWriter, r *http.Request) {
	if s.app.Quit == nil {
		writeErr(w, 404, errors.New("quit not supported"))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
	go s.app.Quit()
}

// postShow is used by a second launch of the app to surface this instance.
func (s *Server) postShow(w http.ResponseWriter, r *http.Request) {
	if s.app.Show != nil {
		go s.app.Show()
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// events streams engine events as SSE. A full status snapshot is sent first.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	send := func(ev engine.Event) bool {
		raw, err := json.Marshal(ev.Data)
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, raw); err != nil {
			return false
		}
		fl.Flush()
		return true
	}

	if !send(engine.Event{Type: "status", Data: s.eng.Status()}) ||
		!send(engine.Event{Type: "setup", Data: s.setup.State()}) {
		return
	}
	ch, unsub := s.eng.Subscribe()
	defer unsub()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.ctx.Done(): // app shutting down: don't hold up srv.Shutdown
			return
		case ev := <-ch:
			if ev.Type == "servers" {
				ev.Data = map[string]bool{"changed": true}
			}
			if !send(ev) {
				return
			}
		}
	}
}

// static serves the embedded SPA, falling back to index.html for app routes.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if _, err := fs.Stat(s.web, path); err != nil {
		path = "index.html"
	}
	http.ServeFileFS(w, r, s.web, path)
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
