package setup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strings"

	"vpngate-client/internal/vpn"
)

const sudoersFile = "/etc/sudoers.d/vpngate-client"

func hasPkexec() bool {
	_, err := exec.LookPath("pkexec")
	return err == nil
}

func missingState() State {
	st := State{Phase: PhaseMissing, Message: "OpenVPN is not installed"}
	switch {
	case os.Geteuid() == 0:
		st.Detail = "Installing it with your system package manager."
		st.Action = "Install OpenVPN"
	case hasPkexec():
		st.Detail = "One-time setup installs OpenVPN with your package manager and lets this app start the tunnel without asking for your password again."
		st.Action = "Install & set up"
	default:
		st.Detail = "Install it with your package manager (e.g. `sudo apt install openvpn`), then run this app with sudo."
	}
	return st
}

func permissionState(bin string) State {
	st := State{Phase: PhasePermission, Message: "Permission needed to create the tunnel", OpenVPN: bin}
	if hasPkexec() {
		st.Detail = "One-time setup lets this app start VPN tunnels without asking for your password every time. You'll be asked for your password once."
		st.Action = "Grant access"
	} else {
		st.Detail = "Run this app with sudo, or add a sudoers rule: `" + currentUser() + " ALL=(root) NOPASSWD: " + bin + "`"
	}
	return st
}

// As root nothing needs consent: install straight away.
func autoFixable(st State) bool {
	return st.Phase == PhaseMissing && os.Geteuid() == 0
}

func prepare(string) error { return nil }

func fix(ctx context.Context, m *Manager) error {
	if os.Geteuid() == 0 {
		return installOpenVPN(ctx, func(line string) { m.progress(PhaseInstalling, "Installing OpenVPN…", -1) })
	}
	if !hasPkexec() {
		return errors.New("pkexec is not available — run this app with sudo instead")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	m.progress(PhaseInstalling, "Waiting for authorization…", -1)
	cmd := exec.CommandContext(ctx, "pkexec", exe, "-setup-helper", "-setup-user", currentUser())
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	var last string
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			last = line
			m.progress(PhaseInstalling, line, -1)
		}
	}
	if err := cmd.Wait(); err != nil {
		var exit *exec.ExitError
		// pkexec: 126 = dialog dismissed / not authorized, 127 = no auth agent.
		if errors.As(err, &exit) && (exit.ExitCode() == 126 || exit.ExitCode() == 127) {
			return errors.New("authorization was cancelled")
		}
		if last != "" {
			return errors.New(last)
		}
		return err
	}
	return nil
}

func currentUser() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return os.Getenv("USER")
}

var validUser = regexp.MustCompile(`^[a-z_][a-z0-9_.-]*\$?$`)

// RunHelper is the privileged half of the Linux setup, run as root through
// pkexec: install OpenVPN if needed and allow username to start it via sudo
// without a password. Progress goes to stdout. Returns the exit code.
func RunHelper(username string) int {
	fail := func(format string, a ...any) int {
		fmt.Printf(format+"\n", a...)
		return 1
	}
	if os.Geteuid() != 0 {
		return fail("setup helper must run as root")
	}
	if !validUser.MatchString(username) {
		return fail("invalid user name %q", username)
	}
	u, err := user.Lookup(username)
	if err != nil {
		return fail("unknown user %q", username)
	}
	// Only grant access to the user who actually asked for it.
	if uid := os.Getenv("PKEXEC_UID"); uid != "" && uid != u.Uid {
		return fail("refusing to grant access to a different user")
	}

	if _, err := vpn.FindOpenVPN(); err != nil {
		fmt.Println("Installing OpenVPN…")
		if err := installOpenVPN(context.Background(), func(string) {}); err != nil {
			return fail("%v", err)
		}
	}
	if _, err := vpn.FindOpenVPN(); err != nil {
		return fail("OpenVPN still not found after install")
	}
	if u.Uid == "0" {
		return 0
	}
	fmt.Println("Granting access…")
	if err := installLauncher(); err != nil {
		return fail("install launcher: %v", err)
	}
	if err := writeSudoers(username); err != nil {
		return fail("%v", err)
	}
	fmt.Println("Done")
	return 0
}

// installLauncher copies the running binary (via /proc/self/exe, i.e. the
// exact file pkexec authorized) to a root-owned location under the
// launcher's name — the name is what restricts it to launcher behavior.
func installLauncher() error {
	src, err := os.Open("/proc/self/exe")
	if err != nil {
		return err
	}
	defer src.Close()
	if err := os.MkdirAll(vpn.LauncherDir, 0o755); err != nil {
		return err
	}
	if err := os.Chown(vpn.LauncherDir, 0, 0); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(vpn.LauncherDir, ".launch-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), vpn.LauncherPath)
}

func writeSudoers(username string) error {
	if _, err := exec.LookPath("sudo"); err != nil {
		return errors.New("sudo is not installed")
	}
	rule := fmt.Sprintf("# Added by VPN Gate Client: lets %s start VPN tunnels without a password prompt.\n"+
		"# The launcher only runs OpenVPN with fixed arguments on a sanitized config.\n"+
		"# Delete this file (and %s) to revoke.\n%s ALL=(root) NOPASSWD: %s\n",
		username, vpn.LauncherDir, username, vpn.LauncherPath)
	if err := os.MkdirAll("/etc/sudoers.d", 0o750); err != nil {
		return err
	}
	tmp := sudoersFile + ".tmp"
	if err := os.WriteFile(tmp, []byte(rule), 0o440); err != nil {
		return err
	}
	if visudo, err := exec.LookPath("visudo"); err == nil {
		if out, err := exec.Command(visudo, "-cf", tmp).CombinedOutput(); err != nil {
			os.Remove(tmp)
			return fmt.Errorf("sudoers validation failed: %s", strings.TrimSpace(string(out)))
		}
	}
	return os.Rename(tmp, sudoersFile)
}

// installOpenVPN uses whichever package manager the distro ships.
func installOpenVPN(ctx context.Context, logf func(string)) error {
	type pm struct {
		bin  string
		cmds [][]string
	}
	managers := []pm{
		{"apt-get", [][]string{{"apt-get", "update", "-qq"}, {"apt-get", "install", "-y", "-qq", "openvpn"}}},
		{"dnf", [][]string{{"dnf", "install", "-y", "openvpn"}}},
		{"yum", [][]string{{"yum", "install", "-y", "openvpn"}}},
		{"pacman", [][]string{{"pacman", "-S", "--noconfirm", "--needed", "openvpn"}}},
		{"zypper", [][]string{{"zypper", "--non-interactive", "install", "openvpn"}}},
		{"apk", [][]string{{"apk", "add", "openvpn"}}},
		{"xbps-install", [][]string{{"xbps-install", "-Sy", "openvpn"}}},
	}
	for _, p := range managers {
		if _, err := exec.LookPath(p.bin); err != nil {
			continue
		}
		for _, args := range p.cmds {
			logf(strings.Join(args, " "))
			cmd := exec.CommandContext(ctx, args[0], args[1:]...)
			cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s failed: %s", strings.Join(args, " "), tail(string(out), 300))
			}
		}
		return nil
	}
	return errors.New("no supported package manager found — install openvpn manually")
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}
