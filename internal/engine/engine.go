// Package engine drives connection lifecycle: ranking servers, trying them in
// order with retries, learning which are dead, and supervising the live tunnel.
package engine

import (
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"vpngate-client/internal/store"
	"vpngate-client/internal/vpn"
	"vpngate-client/internal/vpngate"
)

// ServerView is a vpngate server merged with local stats and derived fields.
type ServerView struct {
	vpngate.Server
	Stats     store.ServerStats `json:"stats"`
	EffScore  float64           `json:"effScore"`
	Health    string            `json:"health"` // new | good | flaky | dead
	SpeedMbps float64           `json:"speedMbps"`
}

// Status is the live engine state pushed to the UI.
type Status struct {
	State       string      `json:"state"` // disconnected | connecting | connected | retrying
	Mode        string      `json:"mode"`  // manual | auto | ""
	Message     string      `json:"message"`
	Server      *ServerView `json:"server,omitempty"`
	PublicIP    string      `json:"publicIp,omitempty"`
	ConnectedAt time.Time   `json:"connectedAt,omitzero"`
	Attempt     int         `json:"attempt,omitempty"`
	Total       int         `json:"total,omitempty"`
	RxBytes     uint64      `json:"rxBytes"`
	TxBytes     uint64      `json:"txBytes"`
	RxRate      float64     `json:"rxRate"` // bytes/sec
	TxRate      float64     `json:"txRate"`
	LastError   string      `json:"lastError,omitempty"`
	ListSize    int         `json:"listSize"`
	ListAge     int64       `json:"listAgeSec"`
}

// Event is what SSE subscribers receive.
type Event struct {
	Type string `json:"type"` // status | log | servers
	Data any    `json:"data"`
}

// LogLine is one entry of the rolling log.
type LogLine struct {
	Time time.Time `json:"time"`
	Line string    `json:"line"`
}

type mode struct {
	manualID  string   // try this server first / only
	countries []string // restrict auto candidates
	loop      bool     // keep retrying rounds forever (auto mode)
}

type Engine struct {
	st      *store.Store
	dataDir string

	mu        sync.Mutex
	servers   []vpngate.Server
	fetchedAt time.Time
	status    Status
	conn      *vpn.Conn
	loopStop  context.CancelFunc
	loopDone  chan struct{}
	subs      map[chan Event]struct{}
	logs      []LogLine
}

func New(st *store.Store, dataDir string) *Engine {
	return &Engine{
		st:      st,
		dataDir: dataDir,
		status:  Status{State: "disconnected", Message: "not connected"},
		subs:    map[chan Event]struct{}{},
	}
}

// Start loads the server list (cache first for instant boot), begins periodic
// refresh, and kicks off auto-connect if configured. Blocks only briefly.
func (e *Engine) Start(ctx context.Context) {
	if servers, mod, err := vpngate.LoadCache(e.dataDir); err == nil {
		e.setServers(servers, mod)
		e.logf("loaded %d servers from cache (%s old)", len(servers), time.Since(mod).Round(time.Minute))
	}
	go func() {
		if err := e.RefreshServers(ctx, false); err != nil {
			e.logf("server list refresh failed: %v", err)
		}
		if e.st.Settings().AutoConnect {
			e.logf("auto-connect on startup enabled")
			_ = e.AutoConnect(e.st.Settings().Countries)
		}
		t := time.NewTicker(30 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := e.RefreshServers(ctx, true); err != nil {
					e.logf("periodic refresh failed: %v", err)
				}
			}
		}
	}()
}

// RefreshServers re-downloads the list unless a recent one is already loaded.
func (e *Engine) RefreshServers(ctx context.Context, force bool) error {
	e.mu.Lock()
	fresh := time.Since(e.fetchedAt) < 10*time.Minute && len(e.servers) > 0
	e.mu.Unlock()
	if fresh && !force {
		return nil
	}
	servers, err := vpngate.Fetch(ctx, e.dataDir)
	if err != nil {
		return err
	}
	e.setServers(servers, time.Now())
	e.logf("server list refreshed: %d servers", len(servers))
	e.broadcast(Event{Type: "servers"})
	return nil
}

func (e *Engine) setServers(servers []vpngate.Server, at time.Time) {
	e.mu.Lock()
	e.servers = servers
	e.fetchedAt = at
	e.status.ListSize = len(servers)
	e.mu.Unlock()
	e.pushStatus()
}

// Servers returns the merged, scored view of every known server.
func (e *Engine) Servers() []ServerView {
	e.mu.Lock()
	servers := e.servers
	e.mu.Unlock()
	set := e.st.Settings()
	out := make([]ServerView, 0, len(servers))
	for _, s := range servers {
		out = append(out, e.view(s, set))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EffScore > out[j].EffScore })
	return out
}

// Recommendations returns the top non-dead servers, optionally country-scoped.
func (e *Engine) Recommendations(n int, countries []string) []ServerView {
	all := e.Servers()
	var out []ServerView
	for _, s := range all {
		if s.Health == "dead" || !matchCountry(s, countries) {
			continue
		}
		out = append(out, s)
		if len(out) >= n {
			break
		}
	}
	return out
}

func (e *Engine) view(s vpngate.Server, set store.Settings) ServerView {
	st := e.st.Stats(s.Key())
	v := ServerView{
		Server:    s,
		Stats:     st,
		SpeedMbps: math.Round(float64(s.SpeedBps)*8/1e6*10) / 10,
	}
	v.EffScore = effScore(s, st)
	switch {
	case st.ConsecutiveFails >= set.DeadThreshold:
		v.Health = "dead"
	case st.Successes > 0 && st.ConsecutiveFails == 0:
		v.Health = "good"
	case st.Failures > 0:
		v.Health = "flaky"
	default:
		v.Health = "new"
	}
	return v
}

// effScore blends VPN Gate's published metrics with locally learned history
// into a single 0-ish..1.5-ish ranking value.
func effScore(s vpngate.Server, st store.ServerStats) float64 {
	speed := math.Min(float64(s.SpeedBps)/50e6, 1) // 50 MB/s caps out
	vg := math.Min(float64(s.Score)/1e6, 1)
	ping := 1.0
	if s.PingMs > 0 {
		ping = 1 - math.Min(float64(s.PingMs)/300, 1)
	}
	score := 0.45*speed + 0.2*vg + 0.15*ping
	if st.Successes > 0 {
		score += 0.2 * math.Min(float64(st.Successes)/5, 1)
	}
	score -= 0.12 * float64(st.ConsecutiveFails)
	if st.LastLatencyMs > 0 {
		score += 0.05
	} else if st.LastLatencyMs < 0 {
		score -= 0.15
	}
	if st.Favorite {
		score += 0.3
	}
	return math.Round(score*1000) / 1000
}

func matchCountry(s ServerView, countries []string) bool {
	if len(countries) == 0 {
		return true
	}
	for _, c := range countries {
		if strings.EqualFold(c, s.CountryShort) {
			return true
		}
	}
	return false
}

// ---- connection control ----

// Connect tries one specific server (falling back to auto-reconnect within
// its country if the link later drops).
func (e *Engine) Connect(id string) error {
	s, ok := e.serverByID(id)
	if !ok {
		return fmt.Errorf("unknown server id %q", id)
	}
	e.startLoop(mode{manualID: id, countries: []string{s.CountryShort}, loop: false})
	return nil
}

// AutoConnect walks ranked candidates in the given countries (empty = all)
// and keeps retrying rounds until connected or cancelled.
func (e *Engine) AutoConnect(countries []string) error {
	set := e.st.Settings()
	set.Countries = normCountries(countries)
	e.st.SetSettings(set)
	e.startLoop(mode{countries: set.Countries, loop: true})
	return nil
}

// Disconnect cancels any running loop and tears down the tunnel.
func (e *Engine) Disconnect() {
	e.stopLoop()
	e.setStatus(func(s *Status) {
		*s = Status{State: "disconnected", Message: "disconnected by user", ListSize: s.ListSize, ListAge: s.ListAge}
	})
	e.logf("disconnected by user")
}

func (e *Engine) stopLoop() {
	e.mu.Lock()
	cancel, done := e.loopStop, e.loopDone
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (e *Engine) startLoop(m mode) {
	e.stopLoop()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	e.mu.Lock()
	e.loopStop, e.loopDone = cancel, done
	e.mu.Unlock()
	go func() {
		defer close(done)
		e.runLoop(ctx, m)
	}()
}

func (e *Engine) serverByID(id string) (vpngate.Server, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.servers {
		if s.ID == id {
			return s, true
		}
	}
	return vpngate.Server{}, false
}

type attemptResult int

const (
	resFailed attemptResult = iota // never reached connected
	resDropped                     // connected, then link died
	resCancelled                   // ctx cancelled (user or shutdown)
)

func (e *Engine) runLoop(ctx context.Context, m mode) {
	// An environment problem (no openvpn, no root) would otherwise burn a
	// "failure" onto every candidate — bail out with the real cause instead.
	if err := vpn.Preflight(); err != nil {
		e.finish(err.Error())
		return
	}
	for _, w := range vpn.DetectConflicts() {
		e.logf("WARNING: %s", w)
	}
	round := 0
	manualFirst := m.manualID
rounds:
	for {
		cands := e.buildCandidates(m, manualFirst)
		if len(cands) == 0 {
			e.finish("no servers match the current filter — try refreshing the list or widening countries")
			return
		}
		for i, c := range cands {
			if ctx.Err() != nil {
				return
			}
			res := e.tryServer(ctx, c, i+1, len(cands), m)
			switch res {
			case resCancelled:
				return
			case resDropped:
				if !e.st.Settings().AutoReconnect {
					e.finish("connection dropped (auto-reconnect is off)")
					return
				}
				e.logf("connection dropped — reconnecting")
				manualFirst = "" // re-rank from scratch, don't pin the dropped server
				round = 0
				continue rounds
			case resFailed:
				continue
			}
		}
		// Round exhausted.
		if !m.loop {
			e.finish("could not connect to the selected server")
			return
		}
		round++
		wait := time.Duration(math.Min(float64(10*round), 60)) * time.Second
		e.setStatus(func(s *Status) {
			s.State = "retrying"
			s.Server = nil
			s.Message = fmt.Sprintf("all %d candidates failed — refreshing list, retrying in %s (round %d)", len(cands), wait, round)
		})
		e.logf("round %d exhausted (%d candidates) — retrying in %s", round, len(cands), wait)
		_ = e.RefreshServers(ctx, true)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (e *Engine) finish(msg string) {
	e.setStatus(func(s *Status) {
		s.State = "disconnected"
		s.Mode = ""
		s.Server = nil
		s.Message = msg
	})
	e.logf("%s", msg)
}

// buildCandidates ranks eligible servers for one connection round.
// Dead servers inside their cooldown window are excluded entirely; if nothing
// else is left they are re-admitted, oldest failure first, so a fully "dead"
// region still gets slowly re-probed.
func (e *Engine) buildCandidates(m mode, manualFirst string) []ServerView {
	set := e.st.Settings()
	all := e.Servers() // already sorted by EffScore desc
	cooldown := time.Duration(set.DeadCooldownMin) * time.Minute

	var alive, cooling []ServerView
	var pinned *ServerView
	for _, s := range all {
		if !matchCountry(s, m.countries) && s.ID != manualFirst {
			continue
		}
		if s.ID == manualFirst {
			sc := s
			pinned = &sc
			continue
		}
		if s.Health == "dead" && time.Since(s.Stats.LastFailure) < cooldown {
			cooling = append(cooling, s)
			continue
		}
		alive = append(alive, s)
	}
	if len(alive) == 0 && pinned == nil {
		sort.Slice(cooling, func(i, j int) bool {
			return cooling[i].Stats.LastFailure.Before(cooling[j].Stats.LastFailure)
		})
		alive = cooling
	}
	if len(alive) > set.MaxAutoCandidates {
		alive = alive[:set.MaxAutoCandidates]
	}
	if pinned != nil {
		alive = append([]ServerView{*pinned}, alive...)
		if !m.loop {
			return alive[:1] // manual first attempt: just the chosen server
		}
	}
	return alive
}

func (e *Engine) tryServer(ctx context.Context, c ServerView, attempt, total int, m mode) attemptResult {
	set := e.st.Settings()
	key := c.Key()
	label := fmt.Sprintf("%s (%s, %s:%d/%s)", c.HostName, c.CountryShort, c.IP, c.Port, c.Proto)
	modeName := "auto"
	if m.manualID != "" && attempt == 1 {
		modeName = "manual"
	}

	e.setStatus(func(s *Status) {
		s.State = "connecting"
		s.Mode = modeName
		sc := c
		s.Server = &sc
		s.Attempt, s.Total = attempt, total
		s.Message = fmt.Sprintf("connecting to %s (attempt %d/%d)", c.HostName, attempt, total)
		s.PublicIP = ""
		s.RxBytes, s.TxBytes, s.RxRate, s.TxRate = 0, 0, 0, 0
	})
	e.logf("[%d/%d] trying %s", attempt, total, label)

	// Cheap reachability probe first (TCP endpoints only).
	if set.PrecheckTCP && c.Proto == "tcp" {
		lat := vpn.TCPPing(c.IP, c.Port, 2500*time.Millisecond)
		e.st.RecordLatency(key, lat)
		if lat < 0 {
			e.st.RecordFailure(key)
			e.logf("precheck: %s unreachable, skipping", label)
			return resFailed
		}
		e.logf("precheck: %s reachable in %dms", label, lat)
	}

	cfg, err := c.Config()
	if err != nil {
		e.st.RecordFailure(key)
		e.logf("bad config for %s: %v", label, err)
		return resFailed
	}

	start := time.Now()
	var rxPrev, txPrev uint64
	var prevAt time.Time
	conn, err := vpn.Start(cfg, vpn.Options{
		ManageDNS: set.ManageDNS,
		Logf:      func(line string) { e.pushLog("[ovpn] " + line) },
		OnBytes: func(rx, tx uint64) {
			now := time.Now()
			e.setStatusQuiet(func(s *Status) {
				if s.State != "connected" {
					return
				}
				if !prevAt.IsZero() {
					dt := now.Sub(prevAt).Seconds()
					if dt > 0 {
						s.RxRate = float64(rx-rxPrev) / dt
						s.TxRate = float64(tx-txPrev) / dt
					}
				}
				s.RxBytes, s.TxBytes = rx, tx
				rxPrev, txPrev, prevAt = rx, tx, now
			})
			e.pushStatus()
		},
	})
	if err != nil {
		e.st.RecordFailure(key)
		e.setStatus(func(s *Status) { s.LastError = err.Error() })
		e.logf("launch failed: %v", err)
		return resFailed
	}

	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(set.ConnectTimeoutSec)*time.Second)
	err = conn.WaitConnected(waitCtx)
	cancel()
	if err != nil {
		conn.Stop()
		if ctx.Err() != nil {
			return resCancelled
		}
		e.st.RecordFailure(key)
		e.logf("connect failed for %s: %v", label, err)
		return resFailed
	}

	// Tunnel is up — make sure it actually routes traffic before declaring victory.
	ip, err := publicIP(ctx)
	if err != nil {
		if ctx.Err() != nil {
			conn.Stop()
			return resCancelled
		}
		e.logf("%s: tunnel up but no internet through it (%v) — marking failed", label, err)
		conn.Stop()
		e.st.RecordFailure(key)
		return resFailed
	}

	connectMs := time.Since(start).Milliseconds()
	e.st.RecordSuccess(key, connectMs)
	e.mu.Lock()
	e.conn = conn
	e.mu.Unlock()
	e.setStatus(func(s *Status) {
		s.State = "connected"
		s.Message = fmt.Sprintf("connected to %s in %.1fs", c.HostName, float64(connectMs)/1000)
		s.PublicIP = ip
		s.ConnectedAt = time.Now()
		s.LastError = ""
		sc := e.view(c.Server, set)
		s.Server = &sc
	})
	e.logf("connected to %s — public IP %s (%.1fs)", label, ip, float64(connectMs)/1000)

	select {
	case <-ctx.Done():
		conn.Stop()
		e.clearConn()
		return resCancelled
	case <-conn.Done():
		e.clearConn()
		e.st.RecordFailure(key) // a drop counts against stability
		e.logf("%s: connection lost (%s)", label, conn.LastError())
		return resDropped
	}
}

func (e *Engine) clearConn() {
	e.mu.Lock()
	e.conn = nil
	e.mu.Unlock()
}

// publicIP fetches our egress IP through the (now default-routed) tunnel.
// The first endpoint is an IP literal on purpose: local DNS handoff to the
// VPN's resolver is often flaky, and a DNS hiccup must not condemn a server
// that routes traffic fine.
func publicIP(ctx context.Context) (string, error) {
	client := &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialContext:       (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		},
	}
	var lastErr error
	for _, url := range []string{
		"https://1.1.1.1/cdn-cgi/trace",
		"https://api.ipify.org",
		"http://checkip.amazonaws.com",
	} {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}
		body := strings.TrimSpace(string(b))
		// cdn-cgi/trace returns key=value lines; the others return a bare IP.
		if ip := extractIP(body); ip != "" {
			return ip, nil
		}
		lastErr = fmt.Errorf("bad response %.40q", body)
	}
	return "", lastErr
}

func extractIP(body string) string {
	if net.ParseIP(body) != nil {
		return body
	}
	for _, line := range strings.Split(body, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "ip="); ok {
			if net.ParseIP(rest) != nil {
				return rest
			}
		}
	}
	return ""
}

// PingSweep probes TCP reachability of the given servers (or the top-ranked
// ones if ids is empty) concurrently and records latencies.
func (e *Engine) PingSweep(ctx context.Context, ids []string) int {
	all := e.Servers()
	var targets []ServerView
	if len(ids) > 0 {
		want := map[string]bool{}
		for _, id := range ids {
			want[id] = true
		}
		for _, s := range all {
			if want[s.ID] && s.Proto == "tcp" {
				targets = append(targets, s)
			}
		}
	} else {
		for _, s := range all {
			if s.Proto == "tcp" {
				targets = append(targets, s)
			}
			if len(targets) >= 60 {
				break
			}
		}
	}
	sem := make(chan struct{}, 20)
	var wg sync.WaitGroup
	for _, s := range targets {
		wg.Add(1)
		go func(s ServerView) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			lat := vpn.TCPPing(s.IP, s.Port, 2500*time.Millisecond)
			e.st.RecordLatency(s.Key(), lat)
		}(s)
	}
	wg.Wait()
	e.broadcast(Event{Type: "servers"})
	e.logf("reachability sweep finished: %d servers probed", len(targets))
	return len(targets)
}

func (e *Engine) ToggleFavorite(id string) error {
	s, ok := e.serverByID(id)
	if !ok {
		return fmt.Errorf("unknown server id %q", id)
	}
	e.st.ToggleFavorite(s.Key())
	e.broadcast(Event{Type: "servers"})
	return nil
}

// ---- status & events ----

func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.status
	if !e.fetchedAt.IsZero() {
		s.ListAge = int64(time.Since(e.fetchedAt).Seconds())
	}
	return s
}

func (e *Engine) setStatusQuiet(fn func(*Status)) {
	e.mu.Lock()
	fn(&e.status)
	e.mu.Unlock()
}

func (e *Engine) setStatus(fn func(*Status)) {
	e.setStatusQuiet(fn)
	e.pushStatus()
}

func (e *Engine) pushStatus() {
	e.broadcast(Event{Type: "status", Data: e.Status()})
}

func (e *Engine) logf(format string, args ...any) {
	e.pushLog("[app] " + fmt.Sprintf(format, args...))
}

func (e *Engine) pushLog(line string) {
	entry := LogLine{Time: time.Now(), Line: line}
	e.mu.Lock()
	e.logs = append(e.logs, entry)
	if len(e.logs) > 500 {
		e.logs = e.logs[len(e.logs)-500:]
	}
	e.mu.Unlock()
	e.broadcast(Event{Type: "log", Data: entry})
}

// Logs returns a copy of the rolling log buffer.
func (e *Engine) Logs() []LogLine {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]LogLine, len(e.logs))
	copy(out, e.logs)
	return out
}

// Subscribe registers an event listener; call the returned func to detach.
func (e *Engine) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 128)
	e.mu.Lock()
	e.subs[ch] = struct{}{}
	e.mu.Unlock()
	return ch, func() {
		e.mu.Lock()
		delete(e.subs, ch)
		e.mu.Unlock()
	}
}

func (e *Engine) broadcast(ev Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for ch := range e.subs {
		select {
		case ch <- ev:
		default: // slow subscriber: drop rather than block the engine
		}
	}
}

func normCountries(in []string) []string {
	out := []string{}
	for _, c := range in {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}
