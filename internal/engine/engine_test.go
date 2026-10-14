package engine

import (
	"path/filepath"
	"testing"
	"time"

	"vpngate-client/internal/store"
	"vpngate-client/internal/vpngate"
)

func newTestEngine(t *testing.T, servers ...vpngate.Server) *Engine {
	t.Helper()
	st, err := store.Load(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	e := New(st, t.TempDir())
	e.setServers(servers, time.Now())
	return e
}

func srv(id, ip string, speed int64) vpngate.Server {
	return vpngate.Server{ID: id, HostName: id, IP: ip, Proto: "tcp", Port: 443, CountryShort: "JP", SpeedBps: speed}
}

func TestRotatedServersGoLast(t *testing.T) {
	a, b, c := srv("a", "1.1.1.1", 9e6), srv("b", "2.2.2.2", 5e6), srv("c", "3.3.3.3", 1e6)
	e := newTestEngine(t, a, b, c)

	got := e.buildCandidates(mode{loop: true}, "", map[string]time.Time{a.Key(): time.Now()})
	if len(got) != 3 || got[0].ID != "b" || got[2].ID != "a" {
		t.Fatalf("order = %v, want b, c, then the rotated-away a last", ids(got))
	}

	// Long after the window, a is back to normal ranking.
	got = e.buildCandidates(mode{loop: true}, "", map[string]time.Time{a.Key(): time.Now().Add(-recentRotationWindow - time.Minute)})
	if got[0].ID != "a" {
		t.Fatalf("order = %v, want a first again", ids(got))
	}
}

func TestRotationTarget(t *testing.T) {
	a, b := srv("a", "1.1.1.1", 9e6), srv("b", "2.2.2.2", 5e6)
	e := newTestEngine(t, a, b)
	if !e.hasRotationTarget([]string{"JP"}, a.Key()) {
		t.Fatal("b should be a rotation target")
	}
	if e.hasRotationTarget([]string{"KR"}, a.Key()) {
		t.Fatal("no server in KR, so no target")
	}
	for i := 0; i < e.st.Settings().DeadThreshold; i++ {
		e.st.RecordFailure(b.Key())
	}
	if e.hasRotationTarget(nil, a.Key()) {
		t.Fatal("only a dead alternative exists; rotation must be held")
	}
}

func TestNextRotation(t *testing.T) {
	e := newTestEngine(t)
	set := store.DefaultSettings()
	connected := time.Now().Add(-3 * time.Hour)

	if _, on := e.nextRotation(connected, set); on {
		t.Fatal("rotation scheduled while auto-rotate is off")
	}

	set.AutoRotate = true
	set.RotateIntervalMin = 1 // below the floor; must still be treated as 60
	due, on := e.nextRotation(connected, set)
	if !on || !due.Equal(connected.Add(time.Hour)) {
		t.Fatalf("due = %v (on=%v), want connectedAt+1h", due, on)
	}

	// Enabling auto-rotate on a long-lived connection restarts the clock.
	cfg := e.st.Settings()
	cfg.AutoRotate, cfg.RotateIntervalMin = true, 120
	e.SaveSettings(cfg)
	due, _ = e.nextRotation(connected, e.st.Settings())
	if time.Until(due) < 119*time.Minute {
		t.Fatalf("due in %v, want ~2h from when it was enabled", time.Until(due))
	}
}

func ids(vs []ServerView) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.ID
	}
	return out
}
