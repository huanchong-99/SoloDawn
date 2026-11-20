package identity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

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
	Received   []receivedVector  `json:"received"`
	Pairing    []pairingVector   `json:"pairing"`
	Messages   []messageVector   `json:"pair_messages"`
	BadSecrets []badSecretVector `json:"bad_secrets"`
	Devlogs    []devlogVector    `json:"devlogs"`
	Adopt      []adoptVector     `json:"adopt"`
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
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	MAC    string `json:"mac"`
}

// messageVector is one pair message arriving at the side whose role is me and
// which holds secret. msg is the message as it is on the wire: JSON text, so
// that it can hold an escape that no decoded text gives back.
type messageVector struct {
	Name   string          `json:"name"`
	Me     string          `json:"me"`
	Secret string          `json:"secret"`
	Msg    json.RawMessage `json:"msg"`
	OK     bool            `json:"ok"`
}

// badSecretVector is a text that a person could type and that is no pairing secret.
type badSecretVector struct {
	Name  string `json:"name"`
	Shown string `json:"shown"`
}

// adoptVector asks whether device me, which belongs to no person and has
// paired with peer, adopts the person of a valid log.
type adoptVector struct {
	Name    string           `json:"name"`
	User    string           `json:"user"`
	Me      string           `json:"me"`
	Peer    string           `json:"peer"`
	Entries []proto.Envelope `json:"entries"`
	OK      bool             `json:"ok"`
}

// receivedVector is one envelope arriving at device me, whose clock reads now,
// as a command or as a signalling payload. known says whether the receiver
// listens to the signer. code is "ok" or the code of the first check that fails.
type receivedVector struct {
	Name  string         `json:"name"`
	Kind  string         `json:"kind"`
	Me    string         `json:"me"`
	Now   int64          `json:"now"`
	Known bool           `json:"known"`
	Env   proto.Envelope `json:"env"`
	Code  string         `json:"code"`
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
		{"k in a spelling that is not canonical", proto.Envelope{Payload: plain, Key: respelled(browser.id()), Signature: valid.Signature}, false},
		{"s in a spelling that is not canonical", proto.Envelope{Payload: plain, Key: browser.id(), Signature: respelled(valid.Signature)}, false},
		{"k with padding", proto.Envelope{Payload: plain, Key: browser.id() + "=", Signature: valid.Signature}, false},
		{"s signed by k, beginning with the identity point", proto.Envelope{Payload: plain, Key: browser.id(), Signature: b64.EncodeToString(identitySignature(browser.key, envelopeContext+plain))}, false},
		{"s with the group order added to its second half", proto.Envelope{Payload: plain, Key: browser.id(), Signature: b64.EncodeToString(plusOrder(mustDecode(valid.Signature)))}, false},
		{"k that is 32 bytes and no point of the curve", proto.Envelope{Payload: plain, Key: b64.EncodeToString(offCurve()), Signature: valid.Signature}, false},
	}
	// Under each weak key, a signature made without any private key that an
	// Ed25519 library following RFC 8032 to the letter verifies.
	for _, key := range weakValues() {
		p, sig := forgery(key)
		v.Envelopes = append(v.Envelopes, envelopeVector{"forged under the weak key " + hex.EncodeToString(key),
			proto.Envelope{Payload: p, Key: b64.EncodeToString(key), Signature: b64.EncodeToString(sig)}, false})
	}

	nonce := testID("challenge")
	v.Rendezvous = []challengeVector{{nonce, node.id(), node.key.SignChallenge(nonce)}}

	// shown is what a person copied: the canonical bytes are its upper case
	// without the hyphens. The first is the code as a device displays it. The
	// last name holds text outside ASCII and a line break, which the mac covers
	// as they are.
	side := func(secret Secret, role string, d device) pairSide {
		return pairSide{d.id(), d.kind, d.name, pairOf(secret, role, d.id(), d.kind, d.name).MAC}
	}
	for _, c := range []struct {
		shown, secret string
		a, b          device
	}{
		{"7K2M-9QXD-4F8T-WB3N", "7K2M9QXD4F8TWB3N", browser, node},
		{"0h1j-8kmn-pqrs-tvwz", "0H1J8KMNPQRSTVWZ", node, tablet},
		{"9ZY8XW7VT6SR5QPN", "9ZY8XW7VT6SR5QPN", node, browser},
		{"7k2m9qxd-4f8twb3n", "7K2M9QXD4F8TWB3N", device{tablet.key, "browser", "Zoë 的平板\n第二行 🙂"}, node},
	} {
		secret := Secret(c.secret)
		v.Pairing = append(v.Pairing, pairingVector{
			Shown: c.shown, Secret: c.secret, Room: secret.Room(),
			A: side(secret, "a", c.a), B: side(secret, "b", c.b),
		})
	}
	v.Messages = messageVectors()

	v.BadSecrets = []badSecretVector{
		{"a space for a hyphen", "7K2M 9QXD-4F8T-WB3N"},
		{"the letter I", "7K2M-9QXD-4F8T-WB3I"},
		{"the letter L, in lower case", "7k2m-9qxd-4f8t-wb3l"},
		{"the letter O", "7K2M-9QXD-4F8T-WB3O"},
		{"the letter U", "7K2M-9QXD-4F8T-WB3U"},
		{"one character short", "7K2M-9QXD-4F8T-WB3"},
		{"one character too long", "7K2M-9QXD-4F8T-WB3NN"},
		{"fifteen valid characters and a long s, which a general upper-casing turns into S", "7K2M-9QXD-4F8T-WB3" + string(rune(0x17f))},
		{"fourteen valid characters and a sharp s, which JavaScript's toUpperCase turns into SS", "7K2M-9QXD-4F8T-WB" + string(rune(0xdf))},
		{"a Kelvin sign for a K", "7" + string(rune(0x212a)) + "2M-9QXD-4F8T-WB3N"},
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
		devlogCase("an add signed by a revoked device, and a second add of a revoked key, in its own spelling and in another one", []device{browser},
			yes(genesis), yes(add(browser, 2, tablet)), yes(revoke(browser, 3, tablet)), no(add(tablet, 4, node)), no(add(browser, 4, tablet)),
			no(rewritten(browser, change(browser, 4, "add", tablet), tablet.id(), respelled(tablet.id())))),
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
			no(browser.key.Sign("device")), no(browser.key.Sign("["+entry+"]")), no(browser.key.Sign("null"))),
		devlogCase("dev is not a device id: a weak key, 31 bytes, a label", []device{browser},
			yes(genesis),
			no(rewritten(browser, entry, node.id(), b64.EncodeToString(weakValues()[2]))),
			no(rewritten(browser, entry, node.id(), b64.EncodeToString(weakValues()[0]))),
			no(rewritten(browser, entry, node.id(), b64.EncodeToString(make([]byte, 31)))),
			no(rewritten(browser, entry, node.id(), "workstation"))),
		devlogCase("dev is 32 bytes in their canonical spelling that are no point of the curve: it is added, and can never sign", []device{browser},
			yes(genesis),
			yes(rewritten(browser, entry, node.id(), b64.EncodeToString(offCurve()))),
			no(proto.Envelope{Payload: change(browser, 3, "add", tablet), Key: b64.EncodeToString(offCurve()), Signature: add(browser, 3, tablet).Signature})),
		devlogCase("p is at most 2,048 bytes as UTF-8: 2,049 are refused, 2,048 accepted", []device{browser, node},
			yes(genesis), no(browser.key.Sign(sized(entry, 2049))), yes(browser.key.Sign(sized(entry, 2048)))),
		devlogCase("ver is an integer by its value: 2.0, and 2.0000000000000001, are 2; null, a text and 2.5 are none", []device{browser, node, tablet},
			yes(genesis),
			no(rewritten(browser, entry, `"ver":2`, `"ver":null`)),
			no(rewritten(browser, entry, `"ver":2`, `"ver":"2"`)),
			no(rewritten(browser, entry, `"ver":2`, `"ver":2.5`)),
			no(rewritten(browser, entry, `"ver":2`, `"ver":9007199254740992`)),
			yes(rewritten(browser, entry, `"ver":2`, `"ver":2.0`)),
			yes(rewritten(browser, change(browser, 3, "add", tablet), `"ver":3`, `"ver":3.0000000000000001`))),
		devlogCase("names are exact: VER beside ver is not read, and VER alone is no ver", []device{browser, node},
			yes(genesis),
			no(rewritten(browser, entry, `"ver":2`, `"VER":2`)),
			no(rewritten(browser, entry, `"op":"add"`, `"OP":"add"`)),
			yes(rewritten(browser, entry, `"ver":2`, `"ver":2,"VER":3,"OP":"revoke","DEV":"`+tablet.id()+`"`))),
		devlogCase("an add has a kind that is node or browser and a name that is a string", []device{browser, node},
			yes(genesis),
			no(rewritten(browser, entry, `"kind":"node"`, `"kind":"phone"`)),
			no(rewritten(browser, entry, `"kind":"node",`, ``)),
			no(rewritten(browser, entry, `"kind":"node"`, `"kind":null`)),
			no(rewritten(browser, entry, `,"name":"workstation"`, ``)),
			no(rewritten(browser, entry, `"name":"workstation"`, `"name":null`)),
			no(rewritten(browser, entry, `"name":"workstation"`, `"name":7`)),
			yes(rewritten(browser, entry, `"name":"workstation"`, `"name":""`))),
		devlogCase("what is not read: n, ts and other fields of any type, and kind and name on a revoke", []device{browser},
			yes(genesis),
			yes(rewritten(browser, entry, `"ts":`+fmt.Sprint(ts+2), `"ts":null,"n":null,"to":null,"g":7,"x":1e400`)),
			yes(browser.key.Sign(`{"t":"device","user":"`+browser.id()+`","ver":3,"op":"revoke","dev":"`+node.id()+`","kind":"phone","name":null}`))),
		devlogCase("a revoke of the last valid device", []device{node},
			yes(genesis), no(revoke(browser, 2, browser)), yes(addNode), yes(revoke(node, 3, browser)), no(revoke(node, 4, node)), yes(revoke(node, 4, tablet))),
		devlogCase("what the rules allow: a revoke of a key never added, a second add, a device revoking itself, the first device revoked", []device{node},
			yes(genesis), yes(revoke(browser, 2, stranger)), no(add(browser, 3, stranger)), yes(add(browser, 3, node)), yes(add(browser, 4, node)),
			yes(add(node, 5, tablet)), yes(revoke(tablet, 6, tablet)), no(add(tablet, 7, stranger)), yes(revoke(node, 7, browser)), no(add(browser, 8, tablet))),
	}
	first := []proto.Envelope{genesis, addNode}
	v.Adopt = []adoptVector{
		{"first run: the browser's genesis and its add of the node", browser.id(), node.id(), browser.id(), first, true},
		{"a node added to an existing person by the browser it paired with", browser.id(), stranger.id(), browser.id(),
			append(slices.Clone(first), add(browser, 3, stranger)), true},
		{"a browser added by the node it paired with", browser.id(), tablet.id(), node.id(),
			append(slices.Clone(first), add(node, 3, tablet)), true},
		{"an add of me signed by a device other than peer", browser.id(), tablet.id(), browser.id(),
			append(slices.Clone(first), add(node, 3, tablet)), false},
		{"me revoked later in the log", browser.id(), node.id(), browser.id(),
			append(slices.Clone(first), revoke(browser, 3, node)), false},
		{"peer revoked later in the log", browser.id(), node.id(), browser.id(),
			append(slices.Clone(first), revoke(node, 3, browser)), false},
		{"a log that never adds me", browser.id(), tablet.id(), browser.id(), first, false},
	}

	v.Received = receivedVectors()
	return v
}

// receivedVectors are the checks of section 3 after the first: who signed,
// what the payload holds, whom it is for and when it was signed.
func receivedVectors() []receivedVector {
	me, other, id := node.id(), tablet.id(), testID("offer")
	n := func(text string) string { return `"n":"` + text + `"` } // text as it is written in the JSON
	p := func(fields string) string { return `{"t":"offer",` + n(id) + fields + `}` }
	at := func(off int64) string { return `,"ts":` + fmt.Sprint(ts+off) }
	to := func(device string) string { return `,"to":"` + device + `"` }
	fresh := at(0)
	var out []receivedVector
	add := func(name, kind, payload, code string) {
		out = append(out, receivedVector{name, kind, me, ts, true, browser.key.Sign(payload), code})
	}
	both := func(name, payload, code string) {
		add(name, "signal", payload, code)
		add(name, "command", payload, code)
	}

	both("to the receiver", p(fresh+to(me)), "ok")
	both("to another device", p(fresh+to(other)), "not_for_me")
	both("an empty to is a value, and nobody's device id", p(fresh+`,"to":""`), "not_for_me")
	add("no to: signalling must carry one", "signal", p(fresh), "not_for_me")
	add("no to: a command may have none", "command", p(fresh), "ok")
	both("to null", p(fresh+`,"to":null`), "bad_request")
	both("to a number", p(fresh+`,"to":7`), "bad_request")
	both("g null", p(fresh+to(me)+`,"g":null`), "bad_request")
	both("TO beside to is not read", p(fresh+to(me)+`,"TO":"`+other+`"`), "ok")
	both("to another device, TO the receiver", p(fresh+to(other)+`,"TO":"`+me+`"`), "not_for_me")
	add("TO alone is no to", "signal", p(fresh+`,"TO":"`+me+`"`), "not_for_me")
	add("TO alone is no to", "command", p(fresh+`,"TO":"`+other+`"`), "ok")
	both("to twice: the last counts", p(fresh+to(other)+to(me)), "ok")
	both("a member nobody reads, holding a number beyond a double", p(fresh+to(me)+`,"x":1e400`), "ok")

	both("no ts", p(to(me)), "bad_request")
	both("ts null", p(`,"ts":null`+to(me)), "bad_request")
	both("ts a text", p(`,"ts":"`+fmt.Sprint(ts)+`"`+to(me)), "bad_request")
	both("ts with a fraction", p(`,"ts":`+fmt.Sprint(ts)+`.5`+to(me)), "bad_request")
	both("ts written with .0", p(`,"ts":`+fmt.Sprint(ts)+`.0`+to(me)), "ok")
	both("ts written with an exponent", p(`,"ts":1.7636e12`+to(me)), "ok")
	both("ts written with digits past a double", p(`,"ts":`+fmt.Sprint(ts)+`.00000000000001`+to(me)), "ok")
	both("ts 2^53, one more than the largest integer", p(`,"ts":9007199254740992`+to(me)), "bad_request")
	both("ts 2^53-1, the largest integer", p(`,"ts":9007199254740991`+to(me)), "stale")
	both("ts written 1e20", p(`,"ts":1e20`+to(me)), "bad_request")
	both("TS alone is no ts", p(`,"TS":`+fmt.Sprint(ts)+to(me)), "bad_request")
	both("a long s in the name is no ts", p(`,"t`+string(rune(0x17f))+`":`+fmt.Sprint(ts)+to(me)), "bad_request")
	both("no n", `{"t":"offer"`+fresh+to(me)+`}`, "bad_request")
	both("n null", `{"t":"offer","n":null`+fresh+to(me)+`}`, "bad_request")
	both("no t", `{`+n(id)+fresh+to(me)+`}`, "bad_request")
	both("t a number", `{"t":7,`+n(id)+fresh+to(me)+`}`, "bad_request")
	both("an empty t is a value", `{"t":"",`+n(id)+fresh+to(me)+`}`, "ok")
	both("an empty n is no id", `{"t":"offer",`+n("")+fresh+to(me)+`}`, "bad_request")
	both("n written with the escape of a lone surrogate", `{"t":"offer",`+n(`\ud800`)+fresh+to(me)+`}`, "bad_request")
	both("n one character short of an id", `{"t":"offer",`+n(id[:21])+fresh+to(me)+`}`, "bad_request")
	both("n one character longer than an id", `{"t":"offer",`+n(id+"A")+fresh+to(me)+`}`, "bad_request")
	both("n in a spelling that is not canonical", `{"t":"offer",`+n(respelled(id))+fresh+to(me)+`}`, "bad_request")
	both("n with padding", `{"t":"offer",`+n(id+"==")+fresh+to(me)+`}`, "bad_request")
	both("n followed by a line break", `{"t":"offer",`+n(id+`\n`)+fresh+to(me)+`}`, "bad_request")
	both("n a device id", `{"t":"offer",`+n(other)+fresh+to(me)+`}`, "bad_request")
	both("n sixteen zero bytes", `{"t":"offer",`+n("AAAAAAAAAAAAAAAAAAAAAA")+fresh+to(me)+`}`, "ok")
	both("n sixteen bytes of ones", `{"t":"offer",`+n("_____________________w")+fresh+to(me)+`}`, "ok")
	both("g an id", p(fresh+to(me)+`,"g":"`+testID("group")+`"`), "ok")
	both("an empty g is no id", p(fresh+to(me)+`,"g":""`), "bad_request")
	both("g that is no id", p(fresh+to(me)+`,"g":"Website"`), "bad_request")
	both("g in a spelling that is not canonical", p(fresh+to(me)+`,"g":"`+respelled(testID("group"))+`"`), "bad_request")
	both("G alone is no g", p(fresh+to(me)+`,"G":"Website"`), "ok")
	both("a list", `[`+p(fresh+to(me))+`]`, "bad_request")
	both("null", `null`, "bad_request")
	both("not JSON", `offer`, "bad_request")

	add("exactly two minutes early", "signal", p(at(-120_000)+to(me)), "ok")
	add("exactly two minutes late", "signal", p(at(120_000)+to(me)), "ok")
	add("two minutes and a millisecond early", "signal", p(at(-120_001)+to(me)), "stale")
	add("two minutes and a millisecond late", "signal", p(at(120_001)+to(me)), "stale")
	add("two minutes and a millisecond early", "command", p(at(-120_001)+to(me)), "ok")
	add("exactly 24 hours early", "command", p(at(-86_400_000)+to(me)), "ok")
	add("exactly 24 hours late", "command", p(at(86_400_000)+to(me)), "ok")
	add("24 hours and a millisecond early", "command", p(at(-86_400_001)+to(me)), "stale")
	add("24 hours and a millisecond late", "command", p(at(86_400_001)+to(me)), "stale")

	// The order of the checks: each case fails a later check as well.
	both("for another device and old: not_for_me comes first", p(at(-86_400_001)+to(other)), "not_for_me")
	both("no t, for another device, old: bad_request comes first", `{`+n(id)+at(-86_400_001)+to(other)+`}`, "bad_request")
	both("n no id, for another device, old: bad_request comes first", `{"t":"offer",`+n("offer")+at(-86_400_001)+to(other)+`}`, "bad_request")
	unknown := receivedVector{"a signer the receiver does not listen to, payload not JSON: unknown_device comes first", "command", me, ts, false, stranger.key.Sign("offer"), "unknown_device"}
	unsigned := unknown
	unsigned.Name, unsigned.Env.Payload, unsigned.Code = "a signature that does not hold, from that signer: bad_sig comes first", "offar", "bad_sig"
	return append(out, unknown, unsigned)
}

// messageVectors are pair messages as one side receives them: the browser, in
// role a, wrote them to the node, in role b, unless the case says otherwise.
func messageVectors() []messageVector {
	const secret = Secret("7K2M9QXD4F8TWB3N")
	wire := func(m proto.Pair) json.RawMessage {
		b, err := json.Marshal(m)
		if err != nil {
			panic(err) // a struct of strings always encodes
		}
		return b
	}
	sent := pairOf(secret, "a", browser.id(), browser.kind, browser.name)
	with := func(change func(*proto.Pair)) json.RawMessage {
		m := sent
		change(&m)
		return wire(m)
	}
	weakKey := b64.EncodeToString(weakValues()[2])
	// What a browser reads as a lone surrogate and Go as U+FFFD: both encode
	// it as the three bytes of U+FFFD, so the mac is over those.
	lone := strings.Replace(string(wire(pairOf(secret, "a", browser.id(), "browser", "\ufffd"))), "\ufffd", "\\ud800", 1)
	noName := strings.Replace(string(wire(pairOf(secret, "a", browser.id(), "browser", ""))), `"name":"",`, "", 1)
	if !strings.Contains(lone, `"name":"\ud800"`) || strings.Contains(noName, "name") {
		panic("the pair messages written by hand are not what they are meant to be")
	}
	return []messageVector{
		{"the other side's message", "b", string(secret), wire(sent), true},
		{"the answer of the other side, read by the side that created the secret", "a", string(secret), wire(pairOf(secret, "b", node.id(), node.kind, node.name)), true},
		{"name changed on the way, with the mac it had", "b", string(secret), with(func(m *proto.Pair) { m.Name = "laptop" }), false},
		{"kind changed on the way, with the mac it had", "b", string(secret), with(func(m *proto.Pair) { m.Kind = "node" }), false},
		{"k changed on the way, with the mac it had", "b", string(secret), with(func(m *proto.Pair) { m.Key = tablet.id() }), false},
		{"a message in the receiver's own role", "a", string(secret), wire(sent), false},
		{"a mac made with another secret", "b", string(secret), wire(pairOf("7K2M9QXD4F8TWB3M", "a", browser.id(), browser.kind, browser.name)), false},
		{"k a weak key, with the mac that follows from it", "b", string(secret), wire(pairOf(secret, "a", weakKey, browser.kind, browser.name)), false},
		{"k in a spelling that is not canonical, with the mac that follows from it", "b", string(secret), wire(pairOf(secret, "a", respelled(browser.id()), browser.kind, browser.name)), false},
		{"kind neither node nor browser, with the mac that follows from it", "b", string(secret), wire(pairOf(secret, "a", browser.id(), "phone", browser.name)), false},
		{"name written as the escape of a lone surrogate, with the mac over U+FFFD", "b", string(secret), json.RawMessage(lone), true},
		{"no name, with the mac that an empty name gives", "b", string(secret), json.RawMessage(noName), false},
	}
}

// sized returns the payload of an add with its name made longer, so that the
// payload is exactly size bytes of UTF-8. The filling is two bytes a
// character, so a count of characters or of UTF-16 units is about half of it.
func sized(entry string, size int) string {
	room := size - len(entry)
	if room < 2 {
		panic("the entry is already that long")
	}
	p := strings.Replace(entry, `"name":"`, `"name":"`+strings.Repeat("é", room/2)+strings.Repeat("x", room%2), 1)
	if len(p) != size {
		panic("the entry has no name to fill")
	}
	return p
}

func mustDecode(s string) []byte {
	b, err := b64.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
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
	if len(v.Keys) < 2 || len(v.Envelopes) < 26 || len(v.Rendezvous) < 1 || len(v.Pairing) < 4 || len(v.Messages) < 7 || len(v.BadSecrets) < 6 || len(v.Devlogs) < 17 || len(v.Adopt) < 6 || len(v.Received) < 40 {
		t.Fatalf("cases are missing: %d keys, %d envelopes, %d rendezvous, %d pairing, %d pair_messages, %d bad_secrets, %d devlogs, %d adopt, %d received",
			len(v.Keys), len(v.Envelopes), len(v.Rendezvous), len(v.Pairing), len(v.Messages), len(v.BadSecrets), len(v.Devlogs), len(v.Adopt), len(v.Received))
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
		// Ed25519 signatures are deterministic: where ok is true, k is one of
		// the keys and signing p again with its seed gives s itself; where ok
		// is false and k is one of the keys, it gives another s.
		k, listed := keys[c.Key]
		if c.OK && !listed {
			t.Fatalf("envelope %q: k is not one of the keys", c.Name)
		}
		if listed && (k.Sign(c.Payload) == c.Envelope) != c.OK {
			t.Errorf("envelope %q: signing p with the seed of k gives s %q, ok is %v", c.Name, k.Sign(c.Payload).Signature, c.OK)
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
		a := proto.Pair{Type: "pair", Key: c.A.Device, Kind: c.A.Kind, Name: c.A.Name, Role: "a", MAC: c.A.MAC}
		b := proto.Pair{Type: "pair", Key: c.B.Device, Kind: c.B.Kind, Name: c.B.Name, Role: "b", MAC: c.B.MAC}
		if s.MAC(a) != c.A.MAC || s.MAC(b) != c.B.MAC {
			t.Errorf("pairing %s: mac a %q, mac b %q", c.Shown, s.MAC(a), s.MAC(b))
		}
		if !s.Verify(a, "a") || !s.Verify(b, "b") || s.Verify(a, "b") || s.Verify(b, "a") {
			t.Errorf("pairing %s: each side must verify the other's message and not its own", c.Shown)
		}
	}

	// A message is read as everything from the wire is, and one that cannot
	// be read is ignored like one that is refused.
	for _, c := range v.Messages {
		sender, roles := "a", map[string]bool{"a": true, "b": true}
		if !roles[c.Me] {
			t.Fatalf("pair message %q: me %q", c.Name, c.Me)
		}
		if c.Me == "a" {
			sender = "b"
		}
		var m proto.Pair
		if got := proto.Decode(c.Msg, &m) == nil && Secret(c.Secret).Verify(m, sender); got != c.OK {
			t.Errorf("pair message %q: accepted %v, ok is %v", c.Name, got, c.OK)
		}
	}

	for _, c := range v.BadSecrets {
		if s, err := ParseSecret(c.Shown); err == nil {
			t.Errorf("bad secret, %s: %q was read as %q", c.Name, c.Shown, s)
		}
	}

	for _, c := range v.Adopt {
		l, err := Replay(c.User, c.Entries)
		if err != nil {
			t.Fatalf("adopt %q: the log is not valid: %v", c.Name, err)
		}
		if got := l.Adopts(c.Me, c.Peer); got != c.OK {
			t.Errorf("adopt %q: Adopts gave %v, ok is %v", c.Name, got, c.OK)
		}
	}

	for _, c := range v.Received {
		open, kinds := OpenCommand, map[string]bool{"signal": true, "command": true}
		if !kinds[c.Kind] {
			t.Fatalf("received %q: kind %q", c.Name, c.Kind)
		}
		if c.Kind == "signal" {
			open = OpenSignal
		}
		code := "ok"
		if _, err := open(c.Env, c.Me, func(string) bool { return c.Known }, time.UnixMilli(c.Now)); err != nil {
			code = err.Error()
		}
		if code != c.Code {
			t.Errorf("received %q as a %s: %s, want %s", c.Name, c.Kind, code, c.Code)
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
