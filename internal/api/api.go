// Package api exposes the engine over REST + Server-Sent Events and serves
// the embedded web UI.
package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"vpngate-client/internal/engine"
	"vpngate-client/internal/store"
	"vpngate-client/internal/vpn"
)

type Server struct {
	eng *engine.Engine
	st  *store.Store
	web fs.FS
}

func New(eng *engine.Engine, st *store.Store, web fs.FS) http.Handler {
	s := &Server{eng: eng, st: st, web: web}
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
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("/", s.static)
	return mux
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
	var cfg store.Settings
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeErr(w, 400, err)
		return
	}
	s.st.SetSettings(cfg)
	writeJSON(w, 200, s.st.Settings())
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
	res := map[string]any{"ok": true, "warnings": vpn.DetectConflicts()}
	if err := vpn.Preflight(); err != nil {
		res["ok"] = false
		res["error"] = err.Error()
	}
	writeJSON(w, 200, res)
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

	if !send(engine.Event{Type: "status", Data: s.eng.Status()}) {
		return
	}
	ch, unsub := s.eng.Subscribe()
	defer unsub()
	for {
		select {
		case <-r.Context().Done():
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
