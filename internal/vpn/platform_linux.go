package vpn

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// The launcher is a root-owned copy of this app, installed by the one-time
// setup, that sudo may run without a password. Unlike a sudo rule for
// openvpn itself (which accepts any arguments, e.g. --up scripts, and so is
// as good as root), the launcher only ever starts openvpn with this app's
// fixed arguments on a sanitized config read from stdin.
const (
	LauncherDir  = "/usr/local/libexec/vpngate-client"
	LauncherName = "vpngate-openvpn-launch"
	LauncherPath = LauncherDir + "/" + LauncherName
)

// FindOpenVPN locates the openvpn binary. sbin directories are checked
// explicitly because they are often missing from a regular user's PATH.
func FindOpenVPN() (string, error) {
	if p, err := exec.LookPath("openvpn"); err == nil {
		return p, nil
	}
	for _, p := range []string{"/usr/sbin/openvpn", "/usr/local/sbin/openvpn", "/sbin/openvpn", "/usr/bin/openvpn"} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", ErrNotInstalled
}

// checkPrivileges succeeds when running as root, when the launcher may be
// run via passwordless sudo, or when the user set up a sudo rule for the
// openvpn binary by hand (supported, but the launcher is preferred).
func checkPrivileges(bin string) error {
	if os.Geteuid() == 0 || launcherAllowed() || sudoOpenVPNAllowed(bin) {
		return nil
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return fmt.Errorf("%w: run this app as root", ErrNoPrivileges)
	}
	return fmt.Errorf("%w: run this app with sudo, or use the one-click setup", ErrNoPrivileges)
}

func launcherAllowed() bool {
	if _, err := os.Stat(LauncherPath); err != nil {
		return false
	}
	out, _ := exec.Command("sudo", "-n", LauncherPath, "-check").Output()
	return bytes.Contains(out, []byte("launcher-ok"))
}

func sudoOpenVPNAllowed(bin string) bool {
	if _, err := exec.LookPath("sudo"); err != nil {
		return false
	}
	// `openvpn --version` exits non-zero on some versions, so judge by output.
	out, _ := exec.Command("sudo", "-n", bin, "--version").CombinedOutput()
	return bytes.Contains(out, []byte("OpenVPN"))
}

func prepareCommand(bin, cleaned string, mgmtPort int, manageDNS bool) (*exec.Cmd, string, error) {
	if os.Geteuid() != 0 && launcherAllowed() {
		args := []string{"-n", LauncherPath, "-mgmt-port", fmt.Sprint(mgmtPort)}
		if manageDNS {
			args = append(args, "-dns")
		}
		cmd := exec.Command("sudo", args...)
		cmd.Stdin = strings.NewReader(cleaned)
		return cmd, "", nil
	}
	return directCommand(bin, cleaned, mgmtPort, manageDNS)
}

// IsLauncher reports whether this process is the installed launcher copy.
// It is decided by the executable's name so that the root-runnable copy can
// never be talked into acting as the app (or the setup helper) via flags.
func IsLauncher() bool {
	exe, err := os.Executable()
	return err == nil && filepath.Base(exe) == LauncherName
}

// RunLauncher is the launcher's entire behavior (running as root via sudo):
// read a config from stdin, sanitize it, and run openvpn with fixed args.
func RunLauncher(args []string) int {
	fs := flag.NewFlagSet(LauncherName, flag.ContinueOnError)
	check := fs.Bool("check", false, "report readiness and exit")
	port := fs.Int("mgmt-port", 0, "management port on 127.0.0.1")
	dns := fs.Bool("dns", false, "apply the VPN's DNS via resolv.conf hooks")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return 2
	}
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "launcher must run as root (via sudo)")
		return 1
	}
	if *check {
		fmt.Println("launcher-ok")
		return 0
	}
	if *port < 1024 || *port > 65535 {
		fmt.Fprintln(os.Stderr, "invalid management port")
		return 2
	}
	bin, err := FindOpenVPN()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil {
		return 1
	}
	cleaned, _ := sanitizeConfig(string(raw))
	dir, path, err := writeConfig(cleaned)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)

	// Pdeathsig is tied to the thread that forks; keep it alive.
	runtime.LockOSThread()
	cmd := exec.Command(bin, openvpnArgs(path, *port, *dns)...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		for s := range sig {
			_ = cmd.Process.Signal(s)
		}
	}()
	if err := cmd.Wait(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		return 1
	}
	return 0
}

func command(bin string, args []string) *exec.Cmd {
	if os.Geteuid() == 0 {
		return exec.Command(bin, args...)
	}
	return exec.Command("sudo", append([]string{"-n", bin}, args...)...)
}

func afterStart(*exec.Cmd) {}

// terminate asks the process to exit; sudo relays SIGTERM to openvpn.
func terminate(p *os.Process) { _ = p.Signal(syscall.SIGTERM) }

func platformArgs(manageDNS bool) []string {
	if !manageDNS {
		return nil
	}
	if up, down, ok := dnsScripts(); ok {
		return []string{"--script-security", "2", "--up", up, "--down", down}
	}
	return nil
}

// dnsScripts finds the distro's resolv.conf update hooks, if installed.
func dnsScripts() (up, down string, ok bool) {
	candidates := []string{
		"/etc/openvpn/update-resolv-conf",
		"/etc/openvpn/scripts/update-systemd-resolved",
	}
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, p, true
		}
	}
	return "", "", false
}

// DetectConflicts looks for other VPN/overlay software whose policy routing
// would blackhole an OpenVPN default route. Each finding is a human-readable
// warning for the UI.
func DetectConflicts() []string {
	var out []string
	// Cloudflare WARP: when connected it fills table 65743 with routes that
	// cover the whole internet, and an ip-rule at high priority sends all
	// unmarked traffic there — including OpenVPN's, causing a routing loop.
	if routes := routeTable("65743"); len(routes) > 0 {
		out = append(out,
			"Cloudflare WARP is connected and will blackhole the VPN tunnel — run `warp-cli disconnect` before connecting.")
	}
	// Tailscale (table 52) / Netbird: only a problem when acting as a full
	// default route (exit node), not for plain overlay subnets.
	if hasDefault(routeTable("52")) {
		out = append(out,
			"A Tailscale exit node is active (default route in table 52) — disable it before connecting.")
	}
	if hasDefault(routeTable("netbird")) {
		out = append(out,
			"Netbird is routing all traffic (exit node) — disable that before connecting.")
	}
	return out
}

func routeTable(table string) string {
	raw, err := exec.Command("ip", "route", "show", "table", table).Output()
	if err != nil {
		return ""
	}
	return string(bytes.TrimSpace(raw))
}

func hasDefault(routes string) bool {
	for _, line := range strings.Split(routes, "\n") {
		if strings.HasPrefix(line, "default") || strings.HasPrefix(line, "0.0.0.0/0") ||
			strings.HasPrefix(line, "0.0.0.0/1") {
			return true
		}
	}
	return false
}
