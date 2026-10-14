package vpn

import (
	"strings"
)

// Directives a VPN Gate client profile legitimately needs. Configs come from
// a public list of volunteer servers and OpenVPN runs as root/admin, so
// anything else — scripts (up, route-up, …), plugins, log/status/writepid
// file paths, management, config includes — is dropped rather than trusted.
var allowedDirectives = map[string]bool{
	"client": true, "pull": true, "tls-client": true, "dev": true, "dev-type": true,
	"proto": true, "remote": true, "port": true, "rport": true, "nobind": true,
	"float": true, "remote-random": true, "resolv-retry": true, "connect-retry": true,
	"server-poll-timeout": true, "explicit-exit-notify": true,
	"cipher": true, "data-ciphers": true, "data-ciphers-fallback": true, "ncp-ciphers": true,
	"auth": true, "tls-cipher": true, "tls-ciphersuites": true, "tls-version-min": true,
	"tls-version-max": true, "remote-cert-tls": true, "ns-cert-type": true,
	"verify-x509-name": true, "key-direction": true, "reneg-sec": true, "auth-nocache": true,
	"persist-key": true, "persist-tun": true, "keepalive": true, "ping": true,
	"ping-restart": true, "comp-lzo": true, "compress": true, "allow-compression": true,
	"tun-mtu": true, "mssfix": true, "fragment": true, "sndbuf": true, "rcvbuf": true,
	"route-delay": true, "redirect-gateway": true, "verb": true, "mute": true,
	"mute-replay-warnings": true,
}

// Inline blocks (<ca>…</ca>) carrying key material.
var allowedBlocks = map[string]bool{
	"ca": true, "cert": true, "key": true, "tls-auth": true, "tls-crypt": true, "extra-certs": true,
}

// sanitizeConfig keeps only allowlisted directives and inline key blocks,
// returning the cleaned config and the names of dropped directives.
func sanitizeConfig(text string) (string, []string) {
	var out strings.Builder
	var dropped []string
	block := "" // inside an allowed inline block
	skip := ""  // inside a disallowed inline block
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case block != "":
			out.WriteString(line + "\n")
			if line == "</"+block+">" {
				block = ""
			}
			continue
		case skip != "":
			if line == "</"+skip+">" {
				skip = ""
			}
			continue
		case line == "" || line[0] == '#' || line[0] == ';':
			continue
		case strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") && !strings.HasPrefix(line, "</"):
			name := strings.ToLower(strings.Trim(line, "<>"))
			if allowedBlocks[name] {
				block = name
				out.WriteString("<" + name + ">\n")
			} else {
				skip = name
				dropped = append(dropped, "<"+name+">")
			}
			continue
		}
		name := strings.ToLower(strings.TrimPrefix(strings.Fields(line)[0], "--"))
		if allowedDirectives[name] {
			out.WriteString(line + "\n")
		} else {
			dropped = append(dropped, name)
		}
	}
	return out.String(), dropped
}
