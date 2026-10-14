//go:build !linux && !windows

package vpn

import (
	"fmt"
	"os"
	"os/exec"
)

// Unsupported platforms build so the UI can be developed there, but never
// get far enough to launch a tunnel.

func FindOpenVPN() (string, error) {
	if p, err := exec.LookPath("openvpn"); err == nil {
		return p, nil
	}
	return "", ErrNotInstalled
}

func checkPrivileges(string) error {
	if os.Geteuid() == 0 {
		return nil
	}
	return fmt.Errorf("%w: run this app as root", ErrNoPrivileges)
}

func command(bin string, args []string) *exec.Cmd { return exec.Command(bin, args...) }
func afterStart(*exec.Cmd)                        {}
func terminate(p *os.Process)                     { _ = p.Kill() }
func platformArgs(bool) []string                  { return nil }
func DetectConflicts() []string                   { return nil }

func prepareCommand(bin, cleaned string, mgmtPort int, manageDNS bool) (*exec.Cmd, string, error) {
	return directCommand(bin, cleaned, mgmtPort, manageDNS)
}

// The sudo launcher only exists on Linux.
func IsLauncher() bool         { return false }
func RunLauncher([]string) int { return 2 }
