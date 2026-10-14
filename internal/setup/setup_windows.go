package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"

	"vpngate-client/internal/vpn"
)

// The official, signed OpenVPN community installer. Pinned by hash so a
// compromised mirror can't hand us something else to run as administrator.
// To upgrade: bump the version, then update both hashes from the published
// files (verify their .asc signatures against the OpenVPN security key).
const openvpnVersion = "2.7.8-I001"

var installers = map[string]string{
	"amd64": "1ea61a3056905b90607d4b0f084c04e5cf3043e07cf192d9cd44b0474f29cc49",
	"arm64": "6fc0e8899cc5275ee297fb837bc84aa2bc49f8ef96d7ad93ee3fee1e7163a25c",
}

func installerURL(arch string) string {
	return fmt.Sprintf("https://swupdate.openvpn.net/community/releases/OpenVPN-%s-%s.msi", openvpnVersion, arch)
}

func missingState() State {
	return State{
		Phase:   PhaseMissing,
		Message: "OpenVPN is not installed",
		Detail:  "The app will download and install the official OpenVPN package (about 6 MB).",
		Action:  "Install OpenVPN",
	}
}

func permissionState(bin string) State {
	return State{
		Phase:   PhasePermission,
		Message: "Administrator rights needed",
		Detail:  "Creating a VPN tunnel on Windows requires administrator rights. Restart the app and accept the Windows prompt.",
		OpenVPN: bin,
	}
}

// Installing is silent once we're elevated, so do it without asking.
func autoFixable(st State) bool {
	return st.Phase == PhaseMissing && vpn.IsElevated()
}

func fix(ctx context.Context, m *Manager) error {
	if !vpn.IsElevated() {
		return errors.New("administrator rights are required — restart the app and accept the Windows prompt")
	}
	if _, err := vpn.FindOpenVPN(); err == nil {
		prepared.Store(false) // "Repair": re-run the adapter check
		return nil
	}
	sum, ok := installers[runtime.GOARCH]
	if !ok {
		return fmt.Errorf("no OpenVPN installer for %s — install OpenVPN manually from openvpn.net", runtime.GOARCH)
	}
	dir := filepath.Join(m.dataDir, "downloads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	msi := filepath.Join(dir, fmt.Sprintf("OpenVPN-%s-%s.msi", openvpnVersion, runtime.GOARCH))

	// The data dir is writable by unelevated processes, so the installer is
	// verified through a handle that denies writers, and that handle stays
	// open until msiexec is done — nothing can swap the file in between.
	f, err := openLocked(msi)
	if err != nil || hashOf(f) != sum {
		if f != nil {
			f.Close()
		}
		m.progress(PhaseDownloading, "Downloading OpenVPN…", 0)
		if err := download(ctx, installerURL(runtime.GOARCH), msi, func(f float64) {
			m.progress(PhaseDownloading, "Downloading OpenVPN…", f)
		}); err != nil {
			return fmt.Errorf("download OpenVPN: %w", err)
		}
		if f, err = openLocked(msi); err != nil {
			return err
		}
		if got := hashOf(f); got != sum {
			f.Close()
			os.Remove(msi)
			return fmt.Errorf("downloaded installer failed verification (sha256 %s)", got)
		}
	}
	defer f.Close()

	m.progress(PhaseInstalling, "Installing OpenVPN and the network driver…", -1)
	msiexec := filepath.Join(os.Getenv("SystemRoot"), "System32", "msiexec.exe")
	cmd := exec.CommandContext(ctx, msiexec, "/i", msi, "/qn", "/norestart",
		"ADDLOCAL=OpenVPN,Drivers,Drivers.TAPWindows6",
		"/l*v", filepath.Join(dir, "openvpn-install.log"))
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	err = cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit) && exit.ExitCode() == 3010:
		// Success, reboot recommended — the driver usually works without one.
	case errors.As(err, &exit) && exit.ExitCode() == 1618:
		return errors.New("another installation is in progress — wait for it to finish and try again")
	case errors.As(err, &exit):
		return fmt.Errorf("OpenVPN installer failed with code %d (log: %s)", exit.ExitCode(), filepath.Join(dir, "openvpn-install.log"))
	default:
		return err
	}
	f.Close()
	_ = os.Remove(msi)
	return nil
}

var (
	prepared  atomic.Bool
	prepareMu sync.Mutex
)

// prepare makes sure a TAP-Windows6 adapter exists (OpenVPN can't create one
// itself, and some installs — or other VPN apps — leave none behind).
func prepare(bin string) error {
	if prepared.Load() {
		return nil
	}
	prepareMu.Lock()
	defer prepareMu.Unlock()
	tapctl := filepath.Join(filepath.Dir(bin), "tapctl.exe")
	if _, err := os.Stat(tapctl); err != nil {
		prepared.Store(true) // old/unknown layout: let OpenVPN report problems itself
		return nil
	}
	out, err := hidden(tapctl, "list", "--hwid", `root\tap0901`).Output()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		prepared.Store(true)
		return nil
	}
	if !vpn.IsElevated() {
		return nil // can't create one; permission state is reported separately
	}
	if out, err := hidden(tapctl, "create", "--hwid", `root\tap0901`, "--name", "VPN Gate Client").CombinedOutput(); err != nil {
		return fmt.Errorf("create TAP adapter: %v: %s", err, strings.TrimSpace(string(out)))
	}
	prepared.Store(true)
	return nil
}

func hidden(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}

// openLocked opens p for reading while denying other writers and deleters.
func openLocked(p string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), p), nil
}

func hashOf(f *os.File) string {
	if f == nil {
		return ""
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func download(ctx context.Context, url, dst string, onProgress func(float64)) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	pr := &progressReader{r: resp.Body, total: resp.ContentLength, fn: onProgress}
	_, err = io.Copy(f, pr)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

type progressReader struct {
	r     io.Reader
	total int64
	n     int64
	last  time.Time
	fn    func(float64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.n += int64(n)
	if p.total > 0 && time.Since(p.last) > 150*time.Millisecond {
		p.last = time.Now()
		p.fn(float64(p.n) / float64(p.total))
	}
	return n, err
}

// RunHelper is only used on Linux.
func RunHelper(string) int { return 2 }
