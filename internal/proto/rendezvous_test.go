package proto

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
)

func rendezvousGoldens(t *testing.T, envelope Envelope, pair Pair) {
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
	golden(t, "rv-quota-request", RVFrame{"quota"})
	golden(t, "rv-quota-answer", Quota{"quota", 3250000, 52428800})
	golden(t, "rv-ice-answer", ICEConfig{"ice", ice, 3600})
	env := wireJSON(t, envelope)
	golden(t, "rv-to", Route{"to", host, env})
	golden(t, "rv-from", Route{"from", user, env})
	golden(t, "rv-offline", Offline{"offline", host})
	golden(t, "rv-watch", Watch{"watch", []string{host}})
	golden(t, "rv-presence", Presence{"presence", host, true})
	// The room of the pairing secret shown in envelope.md section 6.
	const room = "23b3731de722f5b7425a41416cf5d30b"
	golden(t, "rv-join", RoomID{"join", room})
	golden(t, "rv-joined", RoomID{"joined", room})
	golden(t, "rv-room", RoomData{RoomID{"room", room}, wireJSON(t, pair)})
}
