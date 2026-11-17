package modules

import (
	"strings"
	"testing"

	"krotek-wireguard-installer/core"
)

func TestFormatWireguardShowOutput(t *testing.T) {
	empty := FormatWireguardShowOutput("")
	if len(empty) != 1 || !strings.Contains(core.StripANSI(empty[0]), "inactive") {
		t.Fatalf("unexpected empty wg show output: %v", empty)
	}

	raw := `interface: wg0
  public key: UF4ZbAJR8Izn4yCaqyHZKzm41Mh3lPYr5oaFy5lBEnc=
  private key: (hidden)
  listening port: 51820

peer: WSV179kBd3iEqSjE4gVpiGrR1UDGD974grRjOS+RwE4=
  preshared key: (hidden)
  allowed ips: 10.8.0.2/32, fd42:42:42::2/128
  latest handshake: 14 seconds ago
  transfer: 1.24 MiB received, 4.56 MiB sent`

	lines := FormatWireguardShowOutput(raw)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "INTERFACE:") || !strings.Contains(joined, "PEER #1:") {
		t.Fatalf("expected INTERFACE and PEER #1 headers in:\n%s", joined)
	}
	if !strings.Contains(joined, core.DIM+"(hidden)"+core.NC) {
		t.Errorf("expected dimmed (hidden) keys in:\n%s", joined)
	}
	if !strings.Contains(joined, core.GREEN+"10.8.0.2/32, fd42:42:42::2/128"+core.NC) {
		t.Errorf("expected green allowed ips in:\n%s", joined)
	}
}
