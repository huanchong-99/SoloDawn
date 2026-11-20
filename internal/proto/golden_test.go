package proto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var update = flag.Bool("update", false, "replace protocol goldens with the typed examples")

func wireJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func jsonValue(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func golden[T any](t *testing.T, name string, want T) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		path := filepath.Join("testdata", name+".json")
		encoded := wireJSON(t, want)
		if *update {
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, encoded, "", "  "); err != nil {
				t.Fatal(err)
			}
			pretty.WriteByte('\n')
			if err := os.WriteFile(path, pretty.Bytes(), 0644); err != nil {
				t.Fatal(err)
			}
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		// Compare objects, not the order of their keys. Signed payload strings
		// inside an object are still compared byte for byte.
		if !reflect.DeepEqual(jsonValue(t, encoded), jsonValue(t, data)) {
			t.Fatalf("encoded %s\ngolden  %s", encoded, data)
		}

		var got T
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if roundTrip := wireJSON(t, got); !reflect.DeepEqual(jsonValue(t, roundTrip), jsonValue(t, encoded)) {
			t.Fatalf("round trip %s\nwant       %s", roundTrip, encoded)
		}
	})
}

const ts int64 = 1763600000000

func id(label string) string {
	h := sha256.Sum256([]byte(label))
	return base64.RawURLEncoding.EncodeToString(h[:16])
}

// Reproducible signers for wire examples, never application keys.
func signer(label string) ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("SoloDawn protocol example: " + label))
	return ed25519.NewKeyFromSeed(seed[:])
}

func device(key ed25519.PrivateKey) string {
	return base64.RawURLEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
}

func signed(key ed25519.PrivateKey, p string) Envelope {
	s := ed25519.Sign(key, []byte("solodawn/1\n"+p))
	return Envelope{Payload: p, Key: device(key), Signature: base64.RawURLEncoding.EncodeToString(s)}
}

var (
	browser = signer("browser")
	node    = signer("node")
	user    = device(browser)
	host    = device(node)
	group   = id("group")
	genesis = DeviceEntry{PayloadHeader{Type: "device", Nonce: id("genesis"), Timestamp: ts - 2}, DeviceChange{user, 1, "add", user, "browser", "phone browser"}}
	addNode = DeviceEntry{PayloadHeader{Type: "device", Nonce: id("add"), Timestamp: ts - 1}, DeviceChange{user, 2, "add", host, "node", "workstation"}}
)

func TestPairingAndSessionGoldens(t *testing.T) {
	signal := func(kind, to string) SignalHeader {
		return SignalHeader{PayloadHeader{Type: kind, Nonce: id(kind), Timestamp: ts, To: to}, id("connection")}
	}
	const sdp = "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\n" +
		"m=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 0.0.0.0\r\na=mid:0\r\na=sctp-port:5000\r\n"
	offer := Offer{signal("offer", host), sdp + "a=ice-ufrag:abc1\r\na=ice-pwd:abcdefghijklmnopqrstuv01\r\na=setup:actpass\r\n" +
		"a=fingerprint:sha-256 98:51:B3:F7:F6:25:1B:69:01:7D:93:D6:F4:A9:0B:2E:3E:27:FF:0C:A9:07:14:1C:82:11:A2:79:98:C9:09:CF\r\n", false}
	golden(t, "offer", offer)
	golden(t, "envelope", signed(browser, string(wireJSON(t, offer))))
	golden(t, "answer", Answer{signal("answer", user), sdp + "a=ice-ufrag:abc2\r\na=ice-pwd:abcdefghijklmnopqrstuv02\r\na=setup:active\r\n" +
		"a=fingerprint:sha-256 AB:9F:14:A9:1B:0B:71:6F:BF:BE:34:DA:42:E0:68:EF:14:36:E4:A6:04:0C:15:22:31:E8:0A:E3:89:92:50:2A\r\n"})
	golden(t, "ice", ICECandidate{signal("ice", host), "candidate:1 1 UDP 2130706431 192.0.2.1 47800 typ host", "0"})
	golden(t, "pair", Pair{"pair", user, "browser", "phone browser", "a", "zBXTlY4O9eFeMUexu7JpBROUooUwjouJpxZt4h8eBI4"})

	golden(t, "device-add", addNode)
	golden(t, "device-revoke", DeviceEntry{PayloadHeader{Type: "device", Nonce: id("revoke"), Timestamp: ts + 1}, DeviceChange{User: user, Version: 3, Op: "revoke", Device: host}})
	golden(t, "devlog", Devlog{"devlog", user, []Envelope{
		signed(browser, string(wireJSON(t, genesis))),
		signed(browser, " "+string(wireJSON(t, addNode))),
	}})
	golden(t, "hello", Hello{Request{"hello", 1}, 1, "browser", "phone browser", []string{}})
	golden(t, "welcome", Welcome{"welcome", Success{1, true}, 1, host, "workstation", "0.3.1", ts, []GroupSummary{{group, "Website", 812}}, true})
}
