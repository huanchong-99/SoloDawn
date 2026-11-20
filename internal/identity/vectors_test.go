package identity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// The shared test vectors of contracts/envelope.md section 8. The web client
// reads the same file, so its sign, verify, pairing and device-log code is
// held to the cases below.
const vectorFile = "../proto/testdata/crypto.json"

var update = flag.Bool("update", false, "rewrite the shared crypto vectors from the cases in vectors_test.go")

type vectors struct {
	Keys       []keyVector       `json:"keys"`
	Envelopes  []envelopeVector  `json:"envelopes"`
	Rendezvous []challengeVector `json:"rendezvous"`
	Pairing    []pairingVector   `json:"pairing"`
	Devlogs    []devlogVector    `json:"devlogs"`
}

type keyVector struct {
	Name        string `json:"name"`
	Seed        string `json:"seed"`
	Device      string `json:"device"`
	Fingerprint string `json:"fingerprint"`
}

type envelopeVector struct {
	Name string `json:"name"`
	proto.Envelope
	OK bool `json:"ok"`
}

type challengeVector struct {
	Nonce     string `json:"nonce"`
	Key       string `json:"k"`
	Signature string `json:"s"`
}

type pairingVector struct {
	Shown  string   `json:"shown"`
	Secret string   `json:"secret"`
	Room   string   `json:"room"`
	A      pairSide `json:"a"`
	B      pairSide `json:"b"`
}

type pairSide struct {
	Device string `json:"device"`
	MAC    string `json:"mac"`
}

type devlogVector struct {
	Name    string        `json:"name"`
	User    string        `json:"user"`
	Entries []entryVector `json:"entries"`
	Valid   []string      `json:"valid"`
}

type entryVector struct {
	Env    proto.Envelope `json:"env"`
	Accept bool           `json:"accept"`
}

var lowerHex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

func yes(env proto.Envelope) entryVector { return entryVector{env, true} }
func no(env proto.Envelope) entryVector  { return entryVector{env, false} }

func devlogCase(name string, valid []device, entries ...entryVector) devlogVector {
	ids := []string{}
	for _, d := range valid {
		ids = append(ids, d.id())
	}
	slices.Sort(ids)
	return devlogVector{name, browser.id(), entries, ids}
}

// The expectations are written out here, not taken from the code under test:
// TestCryptoVectorsHold is what compares them with the implementation.
func cryptoVectors() vectors {
	var v vectors
	for _, name := range []string{"browser", "node", "tablet", "stranger"} {
		seed := sha256.Sum256([]byte("SoloDawn protocol example: " + name))
		k := Key(ed25519.NewKeyFromSeed(seed[:]))
		v.Keys = append(v.Keys, keyVector{name, b64.EncodeToString(seed[:]), k.ID(), k.Fingerprint()})
	}

	plain := payload("", ts)
	valid := browser.key.Sign(plain)
	changed := valid
	changed.Payload = strings.Replace(plain, "hello", "hellp", 1)
	otherKey := valid
	otherKey.Signature = node.key.Sign(plain).Signature
	// In p, the two accented letters are JSON escapes (a backslash, "u" and
	// four digits) and the rest of the non-ASCII text is raw UTF-8; the line
	// break, the quotes and the backslash are escapes too. It is signed as written.
	const u = "\x5cu"
	exact := `{"t":"post","n":"` + testID("exact") + `","ts":1763600000000,"g":"` + testID("group") +
		`","text":"na` + u + `00efve caf` + u + `00e9, 你好 🙂\nShe said \"hi\" \\ <b>&</b>"}`
	var value any
	if err := json.Unmarshal([]byte(exact), &value); err != nil {
		panic(err) // the literal above is JSON
	}
	again, err := json.Marshal(value)
	if err != nil {
		panic(err) // what was decoded from JSON encodes
	}
	reencoded := browser.key.Sign(exact)
	reencoded.Payload = string(again)
	v.Envelopes = []envelopeVector{
		{"valid", valid, true},
		{"one byte of p changed", changed, false},
		{"signed by another key than k", otherKey, false},
		{"non-ASCII text and escaped characters, signed as written", browser.key.Sign(exact), true},
		{"the same JSON value encoded again: other bytes", reencoded, false},
		{"signed with the rendezvous prefix", proto.Envelope{Payload: plain, Key: browser.id(), Signature: browser.key.SignChallenge(plain)}, false},
	}

	nonce := testID("challenge")
	v.Rendezvous = []challengeVector{{nonce, node.id(), node.key.SignChallenge(nonce)}}

	// shown is what a person copied: the canonical bytes are its upper case
	// without the hyphens. The first is the code as a device displays it.
	for _, c := range []struct {
		shown, secret string
		a, b          device
	}{
		{"7K2M-9QXD-4F8T-WB3N", "7K2M9QXD4F8TWB3N", browser, node},
		{"0h1j-8kmn-pqrs-tvwz", "0H1J8KMNPQRSTVWZ", node, tablet},
		{"9ZY8XW7VT6SR5QPN", "9ZY8XW7VT6SR5QPN", node, browser},
	} {
		secret := Secret(c.secret)
		v.Pairing = append(v.Pairing, pairingVector{
			Shown: c.shown, Secret: c.secret, Room: secret.Room(),
			A: pairSide{c.a.id(), secret.MAC("a", c.a.id())},
			B: pairSide{c.b.id(), secret.MAC("b", c.b.id())},
		})
	}

	genesis, addNode := add(browser, 1, browser), add(browser, 2, node)
	tampered := add(browser, 2, node)
	tampered.Payload = strings.Replace(tampered.Payload, "workstation", "workstatiom", 1)
	entry := change(browser, 2, "add", node)
	v.Devlogs = []devlogVector{
		devlogCase("genesis, add a second browser, add a node, revoke the second browser", []device{browser, node},
			yes(genesis), yes(add(browser, 2, tablet)), yes(add(tablet, 3, node)), yes(revoke(node, 4, tablet))),
		devlogCase("an add by a device that is not in the log", []device{browser},
			yes(genesis), no(add(stranger, 2, node)), no(add(stranger, 2, stranger))),
		devlogCase("an add signed by a revoked device, and a second add of a revoked key", []device{browser},
			yes(genesis), yes(add(browser, 2, tablet)), yes(revoke(browser, 3, tablet)), no(add(tablet, 4, node)), no(add(browser, 4, tablet))),
		devlogCase("a gap in ver", []device{browser, node},
			yes(genesis), no(add(browser, 3, node)), no(add(browser, 0, node)), yes(addNode)),
		devlogCase("a second, different entry for a version already accepted; the same entry again is not refused and changes nothing", []device{browser, node},
			yes(genesis), yes(addNode), no(add(browser, 2, tablet)), no(node.key.Sign(addNode.Payload)), yes(addNode), yes(genesis)),
		devlogCase("a first entry whose dev, k and user are not one key", []device{browser},
			no(add(browser, 1, node)), no(add(node, 1, browser)), no(browser.key.Sign(change(node, 1, "add", browser))),
			no(node.key.Sign(change(node, 1, "add", node))), no(revoke(browser, 1, browser)), no(addNode), yes(genesis)),
		devlogCase("a signature that does not cover the entry", []device{browser},
			yes(genesis), no(tampered), no(proto.Envelope{Payload: addNode.Payload, Key: browser.id(), Signature: node.key.Sign(addNode.Payload).Signature})),
		devlogCase("a payload that is not a device-log entry", []device{browser},
			yes(genesis),
			no(browser.key.Sign(strings.Replace(entry, `"t":"device"`, `"t":"post"`, 1))),
			no(browser.key.Sign(strings.Replace(entry, `"op":"add"`, `"op":"rename"`, 1))),
			no(browser.key.Sign("device"))),
		devlogCase("what the rules allow: a revoke of a key never added, a second add, a device revoking itself, the first device revoked", []device{node},
			yes(genesis), yes(revoke(browser, 2, stranger)), no(add(browser, 3, stranger)), yes(add(browser, 3, node)), yes(add(browser, 4, node)),
			yes(add(node, 5, tablet)), yes(revoke(tablet, 6, tablet)), no(add(tablet, 7, stranger)), yes(revoke(node, 7, browser)), no(add(browser, 8, tablet))),
	}
	return v
}

func encodeVectors(t *testing.T, v vectors) []byte {
	t.Helper()
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// The file is exactly what the cases above encode to. A change of the
// implementation that changes a key, a signature, a room or a mac fails here
// until the file is rewritten with -update, which is an agreed wire change.
func TestCryptoVectorFileIsCurrent(t *testing.T) {
	want := encodeVectors(t, cryptoVectors())
	if *update {
		if err := os.WriteFile(vectorFile, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(vectorFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is not what the cases in vectors_test.go give; after an agreed change run: go test ./internal/identity -update", vectorFile)
	}
}

// Reads the file as the web client does and holds the implementation to every case.
func TestCryptoVectorsHold(t *testing.T) {
	data, err := os.ReadFile(vectorFile)
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	if len(v.Keys) < 2 || len(v.Envelopes) < 4 || len(v.Rendezvous) < 1 || len(v.Pairing) < 3 || len(v.Devlogs) < 8 {
		t.Fatalf("section 8 asks for more cases: %d keys, %d envelopes, %d rendezvous, %d pairing, %d devlogs",
			len(v.Keys), len(v.Envelopes), len(v.Rendezvous), len(v.Pairing), len(v.Devlogs))
	}

	keys := map[string]Key{}
	for _, c := range v.Keys {
		seed, err := b64.DecodeString(c.Seed)
		if err != nil || len(seed) != ed25519.SeedSize {
			t.Fatalf("key %s: seed %q", c.Name, c.Seed)
		}
		k := Key(ed25519.NewKeyFromSeed(seed))
		if k.ID() != c.Device || k.Fingerprint() != c.Fingerprint {
			t.Errorf("key %s: device %s, fingerprint %s", c.Name, k.ID(), k.Fingerprint())
		}
		keys[c.Device] = k
	}

	for _, c := range v.Envelopes {
		_, err := OpenCommand(c.Envelope, "", everyone, now)
		if c.OK && err != nil || !c.OK && err != BadSig {
			t.Errorf("envelope %q: OpenCommand gave %v, ok is %v", c.Name, err, c.OK)
		}
		// Ed25519 signatures are deterministic: signing p again with the seed of k gives s itself.
		k, listed := keys[c.Key]
		if !listed {
			t.Fatalf("envelope %q: k is not one of the keys", c.Name)
		}
		if again := k.Sign(c.Payload); (again == c.Envelope) != c.OK {
			t.Errorf("envelope %q: signing p with the seed of k gives s %q, ok is %v", c.Name, again.Signature, c.OK)
		}
	}

	for _, c := range v.Rendezvous {
		if !VerifyChallenge(c.Key, c.Nonce, c.Signature) || VerifyChallenge(c.Key, c.Nonce+"A", c.Signature) {
			t.Errorf("rendezvous nonce %q: the answer does not verify for it alone", c.Nonce)
		}
		// The nonce is signed as the text in the frame, not as the bytes it encodes.
		if k, listed := keys[c.Key]; !listed || k.SignChallenge(c.Nonce) != c.Signature {
			t.Errorf("rendezvous nonce %q: signing it with the seed of k does not give s", c.Nonce)
		}
	}

	for _, c := range v.Pairing {
		s, err := ParseSecret(c.Shown)
		if err != nil || string(s) != c.Secret || s.Room() != c.Room {
			t.Errorf("pairing %s: secret %q, room %q, %v", c.Shown, s, s.Room(), err)
		}
		if c.Secret != strings.ReplaceAll(strings.ToUpper(c.Shown), "-", "") || !lowerHex32.MatchString(c.Room) {
			t.Errorf("pairing %s: secret %q is not its upper case without hyphens, or room %q is not 32 lower-case hex digits", c.Shown, c.Secret, c.Room)
		}
		if s.MAC("a", c.A.Device) != c.A.MAC || s.MAC("b", c.B.Device) != c.B.MAC {
			t.Errorf("pairing %s: mac a %q, mac b %q", c.Shown, s.MAC("a", c.A.Device), s.MAC("b", c.B.Device))
		}
		a := proto.Pair{Type: "pair", Key: c.A.Device, Role: "a", MAC: c.A.MAC}
		b := proto.Pair{Type: "pair", Key: c.B.Device, Role: "b", MAC: c.B.MAC}
		if !s.Verify(a, "a") || !s.Verify(b, "b") || s.Verify(a, "b") || s.Verify(b, "a") {
			t.Errorf("pairing %s: each side must verify the other's message and not its own", c.Shown)
		}
	}

	for _, c := range v.Devlogs {
		l := &Log{User: c.User}
		for i, e := range c.Entries {
			if _, err := l.Apply(e.Env); (err == nil) != e.Accept {
				t.Errorf("devlog %q, entry %d: Apply gave %v, accept is %v", c.Name, i+1, err, e.Accept)
			}
		}
		valid := []string{}
		for _, k := range v.Keys {
			if l.Valid(k.Device) {
				valid = append(valid, k.Device)
			}
		}
		slices.Sort(valid)
		if !slices.Equal(valid, c.Valid) {
			t.Errorf("devlog %q: valid devices %v, want %v", c.Name, valid, c.Valid)
		}
	}
}
