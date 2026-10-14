// Package vpn runs and supervises a single OpenVPN client process.
package vpn

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Environment problems reported by Preflight. Callers (the setup manager)
// use errors.Is to decide how to fix them.
var (
	ErrNotInstalled = errors.New("OpenVPN is not installed")
	ErrNoPrivileges = errors.New("administrator rights are required to create the VPN tunnel")
)

// Options configures one connection attempt.
type Options struct {
	ManageDNS bool
	Logf      func(line string)   // receives every openvpn log line
	OnBytes   func(rx, tx uint64) // cumulative session byte counts, ~every 2s
}

// Conn is a running OpenVPN process.
type Conn struct {
	cmd       *exec.Cmd
	cfgDir    string
	mgmtPort  int
	mgmtMu    sync.Mutex
	mgmtConn  net.Conn
	connected chan struct{}
	done      chan struct{}
	stopOnce  sync.Once
	connOnce  sync.Once

	mu       sync.Mutex
	lastErr  string
	exitErr  error
	stopping bool
}

// Preflight checks that openvpn can actually be launched with enough
// privileges. The error wraps ErrNotInstalled or ErrNoPrivileges.
func Preflight() error {
	bin, err := FindOpenVPN()
	if err != nil {
		return err
	}
	return checkPrivileges(bin)
}

// Start launches openvpn with the given config text. It returns once the
// process is spawned; use WaitConnected to wait for the tunnel.
func Start(configText string, o Options) (*Conn, error) {
	if o.Logf == nil {
		o.Logf = func(string) {}
	}
	bin, err := FindOpenVPN()
	if err != nil {
		return nil, err
	}
	if err := checkPrivileges(bin); err != nil {
		return nil, err
	}

	cleaned, dropped := sanitizeConfig(configText)
	if len(dropped) > 0 {
		o.Logf(fmt.Sprintf("ignoring unsupported config directives: %s", strings.Join(dropped, ", ")))
	}
	mgmtPort, err := freePort()
	if err != nil {
		return nil, err
	}
	cmd, dir, err := prepareCommand(bin, cleaned, mgmtPort, o.ManageDNS)
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("start openvpn: %w", err)
	}
	afterStart(cmd)

	c := &Conn{
		cmd:       cmd,
		cfgDir:    dir,
		mgmtPort:  mgmtPort,
		connected: make(chan struct{}),
		done:      make(chan struct{}),
	}

	go c.scanOutput(stdout, o)
	go c.manage(o)
	go func() {
		err := cmd.Wait()
		c.mu.Lock()
		c.exitErr = err
		c.mu.Unlock()
		c.closeMgmt()
		if dir != "" {
			os.RemoveAll(dir)
		}
		close(c.done)
	}()
	return c, nil
}

// writeConfig stores a (sanitized) config in a fresh private temp dir.
func writeConfig(cleaned string) (dir, path string, err error) {
	dir, err = os.MkdirTemp("", "vpngate-*")
	if err != nil {
		return "", "", err
	}
	path = filepath.Join(dir, "client.ovpn")
	if err := os.WriteFile(path, []byte(cleaned), 0o600); err != nil {
		os.RemoveAll(dir)
		return "", "", err
	}
	return dir, path, nil
}

// directCommand runs openvpn (via command, which may add sudo) on a config
// written to a temp dir that the caller removes when the process exits.
func directCommand(bin, cleaned string, mgmtPort int, manageDNS bool) (*exec.Cmd, string, error) {
	dir, path, err := writeConfig(cleaned)
	if err != nil {
		return nil, "", err
	}
	return command(bin, openvpnArgs(path, mgmtPort, manageDNS)), dir, nil
}

// openvpnArgs is the fixed OpenVPN command line for a client session.
func openvpnArgs(cfgPath string, mgmtPort int, manageDNS bool) []string {
	args := []string{
		"--config", cfgPath,
		"--management", "127.0.0.1", strconv.Itoa(mgmtPort),
		"--verb", "3",
		"--connect-retry-max", "1",
		"--server-poll-timeout", "10",
		"--resolv-retry", "2",
		"--ping", "10",
		"--ping-exit", "45",
		"--mute-replay-warnings",
		"--auth-nocache",
		// Many VPN Gate servers still negotiate CBC ciphers; allow them
		// alongside modern AEAD ciphers so OpenVPN 2.6+ doesn't refuse.
		"--data-ciphers", "AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305:AES-128-CBC:AES-256-CBC",
	}
	return append(args, platformArgs(manageDNS)...)
}

// WaitConnected blocks until the tunnel is up, the process dies, or ctx ends.
func (c *Conn) WaitConnected(ctx context.Context) error {
	select {
	case <-c.connected:
		return nil
	case <-c.done:
		return fmt.Errorf("openvpn exited: %s", c.LastError())
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Done is closed when the openvpn process has exited.
func (c *Conn) Done() <-chan struct{} { return c.done }

// LastError returns the most recent error-ish log line, for diagnostics.
func (c *Conn) LastError() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastErr == "" {
		return "no error details"
	}
	return c.lastErr
}

// Stop terminates the process: SIGTERM via the management interface first
// (works regardless of sudo or platform), then a direct terminate, then kill.
func (c *Conn) Stop() {
	c.stopOnce.Do(func() {
		c.mu.Lock()
		c.stopping = true
		c.mu.Unlock()

		c.mgmtMu.Lock()
		if c.mgmtConn != nil {
			_, _ = c.mgmtConn.Write([]byte("signal SIGTERM\n"))
		}
		c.mgmtMu.Unlock()

		select {
		case <-c.done:
			return
		case <-time.After(3 * time.Second):
		}
		if c.cmd.Process != nil {
			terminate(c.cmd.Process)
		}
		select {
		case <-c.done:
		case <-time.After(7 * time.Second):
			if c.cmd.Process != nil {
				_ = c.cmd.Process.Kill()
			}
		}
	})
	<-c.done
}

func (c *Conn) scanOutput(r interface{ Read([]byte) (int, error) }, o Options) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 256*1024)
	for sc.Scan() {
		line := sc.Text()
		o.Logf(line)
		if strings.Contains(line, "Initialization Sequence Completed") {
			c.connOnce.Do(func() { close(c.connected) })
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "fatal") ||
			strings.Contains(line, "AUTH_FAILED") || strings.Contains(lower, "connection refused") ||
			strings.Contains(lower, "connection reset") || strings.Contains(lower, "exiting") {
			c.mu.Lock()
			c.lastErr = strings.TrimSpace(line)
			c.mu.Unlock()
		}
	}
}

// manage connects to the OpenVPN management socket and streams byte counts.
func (c *Conn) manage(o Options) {
	var conn net.Conn
	var err error
	for i := 0; i < 30; i++ {
		select {
		case <-c.done:
			return
		default:
		}
		conn, err = net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", c.mgmtPort), time.Second)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		return
	}
	c.mgmtMu.Lock()
	c.mgmtConn = conn
	c.mgmtMu.Unlock()

	_, _ = conn.Write([]byte("bytecount 2\n"))
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		line := sc.Text()
		if rest, ok := strings.CutPrefix(line, ">BYTECOUNT:"); ok && o.OnBytes != nil {
			parts := strings.SplitN(strings.TrimSpace(rest), ",", 2)
			if len(parts) == 2 {
				rx, _ := strconv.ParseUint(parts[0], 10, 64)
				tx, _ := strconv.ParseUint(parts[1], 10, 64)
				o.OnBytes(rx, tx)
			}
		}
	}
}

func (c *Conn) closeMgmt() {
	c.mgmtMu.Lock()
	defer c.mgmtMu.Unlock()
	if c.mgmtConn != nil {
		_ = c.mgmtConn.Close()
		c.mgmtConn = nil
	}
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// TCPPing measures TCP connect latency to addr; returns -1 if unreachable.
// UDP servers can't be probed this way, so callers should only use it for
// TCP endpoints (or treat it as best-effort).
func TCPPing(ip string, port int, timeout time.Duration) int64 {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(port)), timeout)
	if err != nil {
		return -1
	}
	conn.Close()
	return time.Since(start).Milliseconds()
}
