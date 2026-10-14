package desktop

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
)

// EnsureElevated is a no-op on Linux: the setup flow grants OpenVPN access
// instead of running the whole app as root.
func EnsureElevated() (relaunched bool) { return false }

// Fatal reports an unrecoverable startup error and exits.
func Fatal(format string, a ...any) { log.Fatalf(format, a...) }

// Show opens the UI in the user's browser when a graphical session exists.
func (s *Shell) Show() {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return // headless / service: the URL is in the log
	}
	if err := openBrowser(s.url); err != nil {
		log.Printf("could not open a browser (%v) — open %s manually", err, s.url)
	}
}

// Close is a no-op: the browser tab isn't ours to close.
func (s *Shell) Close() {}

// RunWindow (the native window) only exists on Windows.
func RunWindow(string, string) int { return 2 }

// openBrowser runs xdg-open as the desktop user — never as root, which
// browsers refuse or mishandle when the app was started with sudo.
func openBrowser(url string) error {
	if os.Geteuid() != 0 {
		return exec.Command("xdg-open", url).Start()
	}
	name := os.Getenv("SUDO_USER")
	if name == "" {
		return fmt.Errorf("running as root without SUDO_USER")
	}
	u, err := user.Lookup(name)
	if err != nil {
		return err
	}
	runtime := "/run/user/" + u.Uid
	env := []string{
		"XDG_RUNTIME_DIR=" + runtime,
		"DBUS_SESSION_BUS_ADDRESS=unix:path=" + runtime + "/bus",
	}
	for _, k := range []string{"DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY"} {
		if v := os.Getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	args := append([]string{"-u", name, "env"}, env...)
	return exec.Command("sudo", append(args, "xdg-open", url)...).Start()
}

// AllowFocus is only needed on Windows.
func AllowFocus() {}
