//go:build !linux && !windows

package setup

import (
	"context"
	"errors"
)

func missingState() State {
	return State{Phase: PhaseMissing, Message: "OpenVPN is not installed", Detail: "Install OpenVPN with your package manager."}
}

func permissionState(bin string) State {
	return State{Phase: PhasePermission, Message: "Run this app as root to create the tunnel", OpenVPN: bin}
}

func autoFixable(State) bool { return false }
func prepare(string) error   { return nil }
func fix(context.Context, *Manager) error {
	return errors.New("automatic setup is not supported on this platform")
}
func RunHelper(string) int { return 2 }
