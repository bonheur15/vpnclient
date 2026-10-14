package desktop

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"vpngate-client/internal/vpn"
)

const windowTitle = "VPN Gate Client"

// The window's message loop and WebView2's COM objects must stay on the
// thread that created them; init runs on the main thread.
func init() { runtime.LockOSThread() }

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxW          = user32.NewProc("MessageBoxW")
	procFindWindowW          = user32.NewProc("FindWindowW")
	procShowWindow           = user32.NewProc("ShowWindow")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procAllowSetForeground   = user32.NewProc("AllowSetForegroundWindow")
	procGetDpiForSystem      = user32.NewProc("GetDpiForSystem")
	procSystemParametersInfo = user32.NewProc("SystemParametersInfoW")
	dwmapi                   = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmSetWindowAttr     = dwmapi.NewProc("DwmSetWindowAttribute")
)

// EnsureElevated relaunches the app through UAC when it isn't elevated (the
// embedded manifest normally makes Windows do this before we even start).
func EnsureElevated() (relaunched bool) {
	if windows.GetCurrentProcessToken().IsElevated() {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	args, _ := windows.UTF16PtrFromString(joinArgs(os.Args[1:]))
	cwd, _ := windows.UTF16PtrFromString(filepath.Dir(exe))
	if err := windows.ShellExecute(0, verb, file, args, cwd, windows.SW_NORMAL); err != nil {
		Fatal("VPN Gate Client needs administrator rights to create the VPN tunnel.\n\n%v", err)
	}
	return true
}

func joinArgs(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = syscall.EscapeArg(a)
	}
	return strings.Join(q, " ")
}

// Fatal shows an error dialog (there is no console to print to) and exits.
func Fatal(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	log.Print(msg)
	text, _ := windows.UTF16PtrFromString(msg)
	title, _ := windows.UTF16PtrFromString(windowTitle)
	const mbIconError = 0x10
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), mbIconError)
	os.Exit(1)
}

// Show opens the app window, or focuses it if it's already open. If the
// WebView2 runtime is missing or the window crashes, it falls back to the
// default browser for the rest of the session.
func (s *Shell) Show() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if s.window != nil {
		focusWindow()
		return
	}
	if s.browserMode || !webview2Installed() {
		s.browserMode = true
		openBrowser(s.url)
		return
	}
	exe, err := os.Executable()
	if err == nil {
		cmd := exec.Command(exe, "-ui-window", s.url, "-data", s.dataDir)
		if err = cmd.Start(); err == nil {
			vpn.KillOnExit(cmd) // no orphaned window if the daemon dies
			s.window = cmd.Process
			go s.watch(cmd)
			return
		}
	}
	log.Printf("could not open app window (%v) — using the browser", err)
	s.browserMode = true
	openBrowser(s.url)
}

func (s *Shell) watch(cmd *exec.Cmd) {
	err := cmd.Wait()
	s.mu.Lock()
	s.window = nil
	closed, onQuit := s.closed, s.onQuit
	if err != nil {
		s.browserMode = true
	}
	s.mu.Unlock()
	if closed {
		return // we killed it ourselves while quitting
	}
	if err == nil {
		// The user closed the window: that's "quit" for a desktop app.
		if onQuit != nil {
			onQuit()
		}
		return
	}
	log.Printf("app window exited (%v) — falling back to the browser", err)
	openBrowser(s.url)
}

// Close shuts the app window when the app quits for another reason.
func (s *Shell) Close() {
	s.mu.Lock()
	p := s.window
	s.closed = true
	s.mu.Unlock()
	if p != nil {
		_ = p.Kill()
	}
}

// openBrowser goes through explorer.exe, which hands the URL to the user's
// already-running shell — so the browser does not inherit our admin rights.
func openBrowser(url string) {
	if err := exec.Command("explorer.exe", url).Start(); err != nil {
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
}

// AllowFocus lets the running instance bring its window to the front. Only
// the foreground process may grant that, so a second launch (which the user
// just started, hence foreground) calls this before asking it to show.
func AllowFocus() {
	const asfwAny = 0xFFFFFFFF
	procAllowSetForeground.Call(asfwAny)
}

func focusWindow() {
	title, _ := windows.UTF16PtrFromString(windowTitle)
	class, _ := windows.UTF16PtrFromString("webview")
	h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)))
	if h == 0 {
		return
	}
	const swRestore = 9
	procShowWindow.Call(h, swRestore)
	procSetForegroundWindow.Call(h)
}

// webview2Installed checks the Evergreen runtime's registration (shipped with
// Windows 11 and pushed to Windows 10 by Windows Update).
func webview2Installed() bool {
	const client = `Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	keys := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\` + client},
		{registry.LOCAL_MACHINE, `SOFTWARE\` + client},
		{registry.CURRENT_USER, `Software\` + client},
	}
	for _, k := range keys {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		pv, _, err := key.GetStringValue("pv")
		key.Close()
		if err == nil && pv != "" && pv != "0.0.0.0" {
			return true
		}
	}
	return false
}

// RunWindow is the entry point of the window child process: it shows the UI
// in a native WebView2 window and returns 0 when the user closes it.
func RunWindow(url, dataDir string) int {
	width, height, minW, minH := windowSize()
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  filepath.Join(dataDir, "webview"),
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  windowTitle,
			Width:  uint(width),
			Height: uint(height),
			IconId: 1, // the app icon embedded by go-winres
			Center: true,
		},
	})
	if w == nil {
		return 3
	}
	defer w.Destroy()
	darkTitleBar(uintptr(w.Window()))
	w.SetSize(minW, minH, webview2.HintMin)
	w.Navigate(url)
	w.Run()
	return 0
}

// windowSize returns the window and minimum sizes in physical pixels. The
// manifest declares per-monitor DPI awareness, so sizes are scaled by the
// display's DPI, then clamped to the work area — the library centers with
// unsigned math, so a window taller than the screen would land off-screen.
func windowSize() (w, h, minW, minH int) {
	scale := 1.0
	if procGetDpiForSystem.Find() == nil { // Windows 10 1607+
		if dpi, _, _ := procGetDpiForSystem.Call(); dpi > 0 {
			scale = float64(dpi) / 96
		}
	}
	w, h = int(1200*scale), int(820*scale)
	minW, minH = int(940*scale), int(640*scale)

	var area struct{ Left, Top, Right, Bottom int32 }
	const spiGetWorkArea = 0x30
	if ok, _, _ := procSystemParametersInfo.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&area)), 0); ok != 0 {
		maxW := int(float64(area.Right-area.Left) * 0.94)
		maxH := int(float64(area.Bottom-area.Top) * 0.94)
		w, h = min(w, maxW), min(h, maxH)
		minW, minH = min(minW, w), min(minH, h)
	}
	return w, h, minW, minH
}

// darkTitleBar matches the window frame to the dark UI (Windows 10 20H1+).
func darkTitleBar(hwnd uintptr) {
	const dwmwaUseImmersiveDarkMode = 20
	on := int32(1)
	procDwmSetWindowAttr.Call(hwnd, dwmwaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&on)), unsafe.Sizeof(on))
}
