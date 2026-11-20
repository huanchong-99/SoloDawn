package node

import (
	"testing"
)

func TestRendezvousListenError(t *testing.T) {
	err := Rendezvous(RendezvousOptions{Domain: "rv", NoRelay: true, Cert: "missing", Key: "missing", Listen: "bad"})
	if err == nil || err.Error() != "listen tcp: address bad: missing port in address" {
		t.Fatalf("Rendezvous returned %v", err)
	}
}
