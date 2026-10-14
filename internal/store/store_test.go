package store

import (
	"path/filepath"
	"testing"
)

func TestRotateIntervalFloor(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ in, want int }{
		{0, MinRotateIntervalMin},
		{5, MinRotateIntervalMin},
		{59, MinRotateIntervalMin},
		{60, 60},
		{180, 180},
		{MaxRotateIntervalMin + 1, MaxRotateIntervalMin},
	} {
		cfg := s.Settings()
		cfg.RotateIntervalMin = tc.in
		s.SetSettings(cfg)
		if got := s.Settings().RotateIntervalMin; got != tc.want {
			t.Errorf("RotateIntervalMin %d: got %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestLoadClampsHandEditedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, _ := Load(path)
	cfg := s.Settings()
	cfg.AutoRotate = true
	s.SetSettings(cfg)
	// Bypass SetSettings, as a hand edit of state.json would.
	s.mu.Lock()
	s.d.Settings.RotateIntervalMin = 1
	s.save()
	s.mu.Unlock()

	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Settings(); got.RotateIntervalMin != MinRotateIntervalMin || !got.AutoRotate {
		t.Fatalf("reloaded settings = %+v, want autoRotate with %d min interval", got, MinRotateIntervalMin)
	}
}
