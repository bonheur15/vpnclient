// Package desktop integrates the daemon with the user's desktop: privilege
// elevation, showing the UI (a native window on Windows, the browser
// elsewhere) and reporting fatal errors where the user can see them.
package desktop

import (
	"os"
	"sync"
)

// Shell owns the UI surface for a running instance.
type Shell struct {
	url     string
	dataDir string
	onQuit  func() // called when the user closes the app window

	mu          sync.Mutex
	window      *os.Process
	browserMode bool
	closed      bool
}

// New creates a shell for the UI served at url. If forceBrowser is set the
// UI always opens in the default browser instead of an app window.
func New(url, dataDir string, forceBrowser bool, onQuit func()) *Shell {
	return &Shell{url: url, dataDir: dataDir, onQuit: onQuit, browserMode: forceBrowser}
}

// URL is where the UI is served.
func (s *Shell) URL() string { return s.url }
