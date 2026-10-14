// Package setup makes sure the machine can actually run a tunnel: OpenVPN is
// installed and we're allowed to launch it. Whatever can be fixed without
// asking is fixed automatically on startup (downloading/installing OpenVPN);
// anything that needs consent is surfaced to the UI as a one-click action.
package setup

import (
	"context"
	"errors"
	"sync"
	"time"

	"vpngate-client/internal/vpn"
)

// Phase is the coarse setup state shown by the UI.
type Phase string

const (
	PhaseChecking    Phase = "checking"
	PhaseReady       Phase = "ready"
	PhaseMissing     Phase = "missing"          // OpenVPN not installed
	PhasePermission  Phase = "needs-permission" // installed, but we can't launch it as root/admin
	PhaseDownloading Phase = "downloading"
	PhaseInstalling  Phase = "installing"
	PhaseError       Phase = "error"
)

// State is pushed to the UI whenever it changes.
type State struct {
	Phase    Phase   `json:"phase"`
	Message  string  `json:"message"`
	Detail   string  `json:"detail,omitempty"`
	Progress float64 `json:"progress"`         // 0..1 while downloading, -1 = indeterminate
	Action   string  `json:"action,omitempty"` // label of the one-click fix; empty = none available
	OpenVPN  string  `json:"openvpn,omitempty"`
}

// Ready reports whether a tunnel can be launched.
func (s State) Ready() bool { return s.Phase == PhaseReady }

type Manager struct {
	dataDir  string
	onChange func(State)

	mu      sync.Mutex
	state   State
	busy    bool
	errFrom Phase // the underlying phase a PhaseError state came from
	ready   chan struct{}
	once    sync.Once
}

func New(dataDir string, onChange func(State)) *Manager {
	if onChange == nil {
		onChange = func(State) {}
	}
	return &Manager{
		dataDir:  dataDir,
		onChange: onChange,
		state:    State{Phase: PhaseChecking, Message: "Checking system…", Progress: -1},
		ready:    make(chan struct{}),
	}
}

func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *Manager) set(s State) {
	m.mu.Lock()
	m.state = s
	m.mu.Unlock()
	m.notify(s)
}

func (m *Manager) notify(s State) {
	if s.Ready() {
		m.once.Do(func() { close(m.ready) })
	}
	m.onChange(s)
}

// progress updates only the message/progress of an in-flight fix.
func (m *Manager) progress(phase Phase, msg string, frac float64) {
	m.set(State{Phase: phase, Message: msg, Progress: frac})
}

// WaitReady blocks until setup has succeeded at least once, or ctx ends.
func (m *Manager) WaitReady(ctx context.Context) error {
	select {
	case <-m.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Start checks the system, applies any fix that needs no user consent, and
// keeps re-checking in the background until ready (so a manual fix by the
// user is picked up without restarting).
func (m *Manager) Start(ctx context.Context) {
	go func() {
		st := m.check()
		if !st.Ready() && autoFixable(st) {
			m.runFix(ctx)
		} else {
			m.set(st)
		}
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for tick := 1; !m.State().Ready(); tick++ {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				// Permission checks spawn sudo, which logs every failed
				// attempt to the auth log; poll those only every 2 minutes.
				if m.State().Phase == PhasePermission && tick%8 != 0 {
					continue
				}
				m.Recheck()
			}
		}
	}()
}

// Recheck re-evaluates the system unless a fix is currently running. A
// failed fix's error (and its details) is kept until something changes.
func (m *Manager) Recheck() State {
	m.mu.Lock()
	busy := m.busy
	m.mu.Unlock()
	if busy {
		return m.State()
	}
	st := m.check() // slow: may spawn sudo/tapctl; a fix could start meanwhile
	m.mu.Lock()
	if m.busy || (m.state.Phase == PhaseError && st.Phase == m.errFrom) {
		cur := m.state
		m.mu.Unlock()
		return cur
	}
	m.state = st
	m.mu.Unlock()
	m.notify(st)
	return st
}

var ErrBusy = errors.New("setup is already running")

// Fix starts the platform fix in the background (the UI follows progress via
// state updates). It may show an OS consent prompt (UAC / polkit).
func (m *Manager) Fix(ctx context.Context) error {
	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		return ErrBusy
	}
	m.mu.Unlock()
	go m.runFix(ctx)
	return nil
}

func (m *Manager) runFix(ctx context.Context) {
	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		return
	}
	m.busy = true
	m.mu.Unlock()

	err := fix(ctx, m)
	defer func() {
		m.mu.Lock()
		m.busy = false
		m.mu.Unlock()
	}()

	st := m.check()
	if err != nil && !st.Ready() {
		m.mu.Lock()
		m.errFrom = st.Phase
		m.mu.Unlock()
		st = State{Phase: PhaseError, Message: "Automatic setup failed", Detail: err.Error(), Action: retryAction(st), OpenVPN: st.OpenVPN}
	}
	m.set(st)
}

// check classifies the current environment.
func (m *Manager) check() State {
	bin, err := vpn.FindOpenVPN()
	if err != nil {
		return missingState()
	}
	if err := vpn.Preflight(); err != nil {
		if errors.Is(err, vpn.ErrNoPrivileges) {
			return permissionState(bin)
		}
		return State{Phase: PhaseError, Message: "OpenVPN can't be started", Detail: err.Error(), OpenVPN: bin}
	}
	if err := prepare(bin); err != nil {
		return State{Phase: PhaseError, Message: "OpenVPN is installed but not usable", Detail: err.Error(), Action: "Repair", OpenVPN: bin}
	}
	return State{Phase: PhaseReady, Message: "Ready", OpenVPN: bin}
}

func retryAction(st State) string {
	if st.Action != "" {
		return st.Action
	}
	return "Try again"
}
