package vpn

import (
	"strings"
	"testing"
)

func TestSanitizeConfig(t *testing.T) {
	in := "dev tun\r\nproto tcp\r\nremote 1.2.3.4 443\r\n# comment\r\ncipher AES-128-CBC\r\n" +
		"script-security 2\r\nup /bin/evil\r\n--plugin /tmp/x.so\r\nlog /etc/passwd\r\nconfig /tmp/other.conf\r\n" +
		"<ca>\r\n-----BEGIN CERTIFICATE-----\r\nAAA\r\n-----END CERTIFICATE-----\r\n</ca>\r\n" +
		"<connection>\r\nremote 6.6.6.6\r\n</connection>\r\n" +
		"<key>\r\nKKK\r\n</key>\r\n"
	got, dropped := sanitizeConfig(in)

	for _, want := range []string{"dev tun\n", "proto tcp\n", "remote 1.2.3.4 443\n", "cipher AES-128-CBC\n",
		"<ca>\n-----BEGIN CERTIFICATE-----\nAAA\n-----END CERTIFICATE-----\n</ca>\n", "<key>\nKKK\n</key>\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("sanitized config is missing %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"script-security", "up ", "plugin", "log ", "config ", "6.6.6.6", "connection"} {
		if strings.Contains(got, bad) {
			t.Errorf("sanitized config still contains %q:\n%s", bad, got)
		}
	}
	if len(dropped) != 6 {
		t.Errorf("dropped = %v, want 6 entries", dropped)
	}
}
