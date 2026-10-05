package vpn

import (
	"bytes"
	"os/exec"
	"strings"
)

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
