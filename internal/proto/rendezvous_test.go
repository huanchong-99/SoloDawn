package proto

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
)

func rendezvousGoldens(t *testing.T, envelope Envelope) {
	t.Helper()
	nonce := id("challenge")
	sig := ed25519.Sign(browser, []byte("solodawn/rv/1\n"+nonce))
	golden(t, "rv-challenge", Challenge{"challenge", nonce})
	golden(t, "rv-auth", Auth{"auth", user, base64.RawURLEncoding.EncodeToString(sig)})
	ice := []ICEServer{
		{URLs: []string{"stun:rv.example:3478"}},
		{[]string{"turn:rv.example:3478?transport=udp", "turns:turn.rv.example:443?transport=tcp"}, "1763603600:" + user, id("credential")},
	}
	golden(t, "rv-ready", ICEConfig{"ready", ice, 3600})
	golden(t, "rv-ice-request", RVFrame{"ice"})
	golden(t, "rv-ice-answer", ICEConfig{"ice", ice, 3600})
	env := wireJSON(t, envelope)
	golden(t, "rv-to", Route{"to", host, env})
	golden(t, "rv-from", Route{"from", user, env})
	golden(t, "rv-offline", Offline{"offline", host})
	golden(t, "rv-watch", Watch{"watch", []string{host}})
	golden(t, "rv-presence", Presence{"presence", host, true})
	const room = "23b3731de722f5b7425a41416cf5d30b"
	golden(t, "rv-join", RoomID{"join", room})
	golden(t, "rv-joined", RoomID{"joined", room})
	// The room and pair MAC for the canonical secret in envelope.md section 6.
	pair := Pair{"pair", user, "browser", "phone browser", "a", "LSa7ZkmFClacGrJ6bsUg1G3yTiycOY3RZf3rskaRwCo"}
	golden(t, "rv-room", RoomData{RoomID{"room", room}, wireJSON(t, pair)})
}
