package vpn

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// FindOpenVPN locates openvpn.exe: the path recorded by the official
// installer first, then the default install dir, then PATH.
func FindOpenVPN() (string, error) {
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\OpenVPN`, registry.QUERY_VALUE); err == nil {
		p, _, err := k.GetStringValue("exe_path")
		k.Close()
		if err == nil && fileExists(p) {
			return p, nil
		}
	}
	for _, env := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)"} {
		if dir := os.Getenv(env); dir != "" {
			if p := filepath.Join(dir, "OpenVPN", "bin", "openvpn.exe"); fileExists(p) {
				return p, nil
			}
		}
	}
	if p, err := exec.LookPath("openvpn.exe"); err == nil {
		return p, nil
	}
	return "", ErrNotInstalled
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// IsElevated reports whether this process holds an elevated admin token,
// which OpenVPN needs to configure the adapter and routes.
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func checkPrivileges(string) error {
	if IsElevated() {
		return nil
	}
	return fmt.Errorf("%w: restart the app as administrator", ErrNoPrivileges)
}

func command(bin string, args []string) *exec.Cmd {
	cmd := exec.Command(bin, args...)
	// openvpn.exe is a console program; without this every connection
	// attempt would flash a console window.
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}

var (
	jobOnce sync.Once
	job     windows.Handle
)

// afterStart puts openvpn in a job object that is killed when this process
// exits, so a crash or forced quit never leaves an orphaned tunnel behind.
func afterStart(cmd *exec.Cmd) {
	jobOnce.Do(func() {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			windows.CloseHandle(h)
			return
		}
		job = h // intentionally never closed: closing it is what kills children
	})
	if job == 0 || cmd.Process == nil {
		return
	}
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return
	}
	_ = windows.AssignProcessToJobObject(job, ph)
	windows.CloseHandle(ph)
}

// KillOnExit ties a helper process (e.g. the UI window) to this process's
// lifetime the same way openvpn is.
func KillOnExit(cmd *exec.Cmd) { afterStart(cmd) }

// terminate: Windows has no SIGTERM; the graceful path is the management
// interface (already tried by Stop), so this is a hard stop.
func terminate(p *os.Process) { _ = p.Kill() }

// platformArgs forces the TAP-Windows6 driver: VPN Gate servers negotiate
// CBC ciphers, which the DCO driver can't carry (2.6+ would fall back on its
// own; this just makes it deterministic). OpenVPN applies pushed DNS servers
// to the adapter itself on Windows, so manageDNS needs no hooks.
func platformArgs(bool) []string {
	return []string{"--disable-dco"}
}

// DetectConflicts has no Windows heuristics yet.
func DetectConflicts() []string { return nil }

func prepareCommand(bin, cleaned string, mgmtPort int, manageDNS bool) (*exec.Cmd, string, error) {
	return directCommand(bin, cleaned, mgmtPort, manageDNS)
}

// The sudo launcher only exists on Linux.
func IsLauncher() bool         { return false }
func RunLauncher([]string) int { return 2 }
