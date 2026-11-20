package identity

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

type device struct {
	key        Key
	kind, name string
}

func (d device) id() string { return d.key.ID() }

// The person of every log in these tests is the browser's.
var (
	browser  = device{testKey("browser"), "browser", "phone browser"}
	node     = device{testKey("node"), "node", "workstation"}
	tablet   = device{testKey("tablet"), "browser", "tablet browser"}
	stranger = device{testKey("stranger"), "node", "unknown computer"}
)

func change(user device, ver int, op string, dev device) string {
	e := proto.DeviceEntry{
		PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: testID(fmt.Sprint(ver, op, dev.name)), Timestamp: ts + int64(ver)},
		DeviceChange:  proto.DeviceChange{User: user.id(), Version: ver, Op: op, Device: dev.id()},
	}
	if op == "add" {
		e.Kind, e.Name = dev.kind, dev.name
	}
	b, err := json.Marshal(e)
	if err != nil {
		panic(err) // a struct of strings and integers always encodes
	}
	return string(b)
}

func add(signer device, ver int, dev device) proto.Envelope {
	return signer.key.Sign(change(browser, ver, "add", dev))
}

func revoke(signer device, ver int, dev device) proto.Envelope {
	return signer.key.Sign(change(browser, ver, "revoke", dev))
}

// apply checks one Apply call: its answer, and that the log grew by exactly
// the entry when it was added and not at all otherwise.
func apply(t *testing.T, l *Log, what string, env proto.Envelope, wantAdded bool, wantErr error) {
	t.Helper()
	before := slices.Clone(l.Entries)
	added, err := l.Apply(env)
	if added != wantAdded || err != wantErr {
		t.Errorf("%s: got added %v, error %v; want added %v, error %v", what, added, err, wantAdded, wantErr)
	}
	want := before
	if wantAdded {
		want = append(want, env)
	}
	if !slices.Equal(l.Entries, want) {
		t.Errorf("%s: the log holds %d entries, want %d", what, len(l.Entries), len(want))
	}
}

func valid(l *Log) string {
	var names []string
	for _, d := range []device{browser, node, tablet, stranger} {
		if l.Valid(d.id()) {
			names = append(names, d.name)
		}
	}
	return strings.Join(names, ", ")
}

func started(t *testing.T) *Log {
	t.Helper()
	l := &Log{User: browser.id()}
	apply(t, l, "genesis", add(browser, 1, browser), true, nil)
	return l
}

func TestLogGrowsByAddAndRevoke(t *testing.T) {
	l := &Log{User: browser.id()}
	if got := valid(l); got != "" {
		t.Fatalf("an empty log has valid devices: %s", got)
	}
	apply(t, l, "genesis", add(browser, 1, browser), true, nil)
	if got := valid(l); got != "phone browser" {
		t.Errorf("after the genesis: valid %q", got)
	}
	// A browser may pad what it signs; the entry is kept in exactly those bytes.
	padded := browser.key.Sign(" " + change(browser, 2, "add", tablet))
	apply(t, l, "add the tablet", padded, true, nil)
	apply(t, l, "the tablet adds the node", add(tablet, 3, node), true, nil)
	if got := valid(l); got != "phone browser, workstation, tablet browser" {
		t.Errorf("after three adds: valid %q", got)
	}
	apply(t, l, "the node revokes the tablet", revoke(node, 4, tablet), true, nil)
	if got := valid(l); got != "phone browser, workstation" {
		t.Errorf("after the revoke: valid %q", got)
	}
	apply(t, l, "the node revokes the first device", revoke(node, 5, browser), true, nil)
	if got := valid(l); got != "workstation" || l.User != browser.id() {
		t.Errorf("after revoking the first device: valid %q, user %q", got, l.User)
	}
}

func TestLogGenesis(t *testing.T) {
	l := &Log{User: browser.id()}
	apply(t, l, "dev is not the signer", add(browser, 1, node), false, errGenesis)
	apply(t, l, "the signer is not the person", add(node, 1, browser), false, errGenesis)
	apply(t, l, "the signer adds itself to another person's log", add(node, 1, node), false, errGenesis)
	apply(t, l, "a revoke", revoke(browser, 1, browser), false, errGenesis)
	apply(t, l, "user names another person", node.key.Sign(change(node, 1, "add", node)), false, errPerson)
	apply(t, l, "user is not the key of dev and signer", browser.key.Sign(change(node, 1, "add", browser)), false, errPerson)
	if got := valid(l); got != "" {
		t.Errorf("refused first entries left valid devices: %s", got)
	}
	apply(t, l, "genesis", add(browser, 1, browser), true, nil)
	apply(t, l, "a later entry for another person", browser.key.Sign(change(node, 2, "add", node)), false, errPerson)
}

func TestLogVersionsHaveNoGaps(t *testing.T) {
	empty := &Log{User: browser.id()}
	apply(t, empty, "version 2 on an empty log", add(browser, 2, browser), false, errGap)
	apply(t, empty, "version 0 on an empty log", add(browser, 0, browser), false, errGap)

	l := started(t)
	apply(t, l, "version 3 after 1", add(browser, 3, node), false, errGap)
	apply(t, l, "version 0", add(browser, 0, node), false, errGap)
	apply(t, l, "version -1", add(browser, -1, node), false, errGap)
	apply(t, l, "version 2", add(browser, 2, node), true, nil)
	apply(t, l, "version 4 after 2", add(browser, 4, tablet), false, errGap)
	apply(t, l, "version 3", add(browser, 3, tablet), true, nil)
}

func TestLogFirstAcceptedEntryWins(t *testing.T) {
	l := started(t)
	second, third := add(browser, 2, node), add(node, 3, tablet)
	apply(t, l, "add the node", second, true, nil)
	apply(t, l, "add the tablet", third, true, nil)

	apply(t, l, "the genesis again", add(browser, 1, browser), false, nil)
	apply(t, l, "entry 2 again", second, false, nil)
	apply(t, l, "the last entry again", third, false, nil)

	apply(t, l, "another entry 1", browser.key.Sign(" "+change(browser, 1, "add", browser)), false, errFork)
	apply(t, l, "another entry 2", add(browser, 2, tablet), false, errFork)
	apply(t, l, "another last entry", revoke(browser, 3, node), false, errFork)
	samePayload := node.key.Sign(second.Payload)
	apply(t, l, "entry 2 signed by another valid device", samePayload, false, errFork)
	if got := valid(l); got != "phone browser, workstation, tablet browser" {
		t.Errorf("valid %q", got)
	}
}

func TestLogSignerMustBeValidBefore(t *testing.T) {
	l := started(t)
	apply(t, l, "a stranger adds the node", add(stranger, 2, node), false, UnknownDevice)
	apply(t, l, "a stranger adds itself", add(stranger, 2, stranger), false, UnknownDevice)
	apply(t, l, "a stranger revokes the browser", revoke(stranger, 2, browser), false, UnknownDevice)
	apply(t, l, "add the tablet", add(browser, 2, tablet), true, nil)
	apply(t, l, "revoke the tablet", revoke(browser, 3, tablet), true, nil)
	apply(t, l, "the revoked tablet adds the node", add(tablet, 4, node), false, UnknownDevice)
	apply(t, l, "the revoked tablet revokes the browser", revoke(tablet, 4, browser), false, UnknownDevice)
	apply(t, l, "add the node", add(browser, 4, node), true, nil)
	apply(t, l, "the node revokes itself", revoke(node, 5, node), true, nil)
	apply(t, l, "the node, revoked by itself, adds the stranger", add(node, 6, stranger), false, UnknownDevice)
	if got := valid(l); got != "phone browser" {
		t.Errorf("valid %q", got)
	}
}

func TestLogRevokedKeyIsNeverAddedAgain(t *testing.T) {
	l := started(t)
	apply(t, l, "add the tablet", add(browser, 2, tablet), true, nil)
	apply(t, l, "add the tablet a second time", add(browser, 3, tablet), true, nil)
	apply(t, l, "revoke the tablet", revoke(browser, 4, tablet), true, nil)
	apply(t, l, "add the revoked tablet", add(browser, 5, tablet), false, errRevoked)
	apply(t, l, "revoke the tablet again", revoke(browser, 5, tablet), true, nil)
	apply(t, l, "add the tablet after two revokes", add(browser, 6, tablet), false, errRevoked)
	apply(t, l, "revoke a key that was never added", revoke(browser, 6, stranger), true, nil)
	apply(t, l, "add that key", add(browser, 7, stranger), false, errRevoked)
	respelledTablet := strings.Replace(change(browser, 7, "add", tablet), tablet.id(), respelled(tablet.id()), 1)
	apply(t, l, "add the revoked tablet in another spelling", browser.key.Sign(respelledTablet), false, BadRequest)
	apply(t, l, "add the node", add(browser, 7, node), true, nil)
	if got := valid(l); got != "phone browser, workstation" {
		t.Errorf("valid %q", got)
	}
}

func TestLogChecksTheSignatureFirst(t *testing.T) {
	l := started(t)
	good := add(browser, 2, node)
	altered := good
	altered.Payload = strings.Replace(good.Payload, "workstation", "workstatiom", 1)
	apply(t, l, "payload changed after signing", altered, false, BadSig)
	claimed := add(stranger, 2, node)
	claimed.Key = browser.id()
	apply(t, l, "signed by a stranger in the browser's name", claimed, false, BadSig)
	unsigned := good
	unsigned.Signature = ""
	apply(t, l, "no signature", unsigned, false, BadSig)
	// Not an entry at all, another person's, and a gap: the signature is still what fails.
	junk := node.key.Sign("junk")
	junk.Payload = "junk!"
	apply(t, l, "altered text that is no entry", junk, false, BadSig)
	apply(t, l, "the entry as signed", good, true, nil)
	apply(t, l, "the altered entry for a version already held", altered, false, BadSig)
}

func TestLogRefusesWhatIsNotAnEntry(t *testing.T) {
	l := started(t)
	entry := change(browser, 2, "add", node)
	for what, p := range map[string]string{
		"text that is not JSON":      "device",
		"a JSON list":                "[" + entry + "]",
		"another payload type":       strings.Replace(entry, `"t":"device"`, `"t":"post"`, 1),
		"no payload type":            strings.Replace(entry, `"t":"device",`, "", 1),
		"an op the log lacks":        strings.Replace(entry, `"op":"add"`, `"op":"rename"`, 1),
		"no op":                      strings.Replace(entry, `"op":"add",`, "", 1),
		"a version that is text":     strings.Replace(entry, `"ver":2`, `"ver":"2"`, 1),
		"a version with a fraction":  strings.Replace(entry, `"ver":2`, `"ver":2.5`, 1),
		"a dev that is no device id": strings.Replace(entry, node.id(), "workstation", 1),
		"no dev":                     strings.Replace(entry, `"dev":"`+node.id()+`",`, "", 1),
	} {
		if p == entry {
			t.Fatalf("%s: the payload was not changed", what)
		}
		apply(t, l, what, browser.key.Sign(p), false, BadRequest)
	}
	apply(t, l, "the entry itself", browser.key.Sign(entry), true, nil)
}

func TestReplay(t *testing.T) {
	entries := []proto.Envelope{add(browser, 1, browser), add(browser, 2, node), revoke(node, 3, browser)}
	l, err := Replay(browser.id(), entries)
	if err != nil {
		t.Fatal(err)
	}
	if l.User != browser.id() || !slices.Equal(l.Entries, entries) || valid(l) != "workstation" {
		t.Errorf("replayed log: user %q, %d entries, valid %q", l.User, len(l.Entries), valid(l))
	}

	empty, err := Replay(browser.id(), nil)
	if err != nil || empty.User != browser.id() || len(empty.Entries) != 0 {
		t.Errorf("empty log: %+v, %v", empty, err)
	}

	for what, c := range map[string]struct {
		user    string
		entries []proto.Envelope
		want    error
	}{
		"a gap":                   {browser.id(), []proto.Envelope{entries[0], entries[2]}, errGap},
		"no genesis":              {browser.id(), entries[1:], errGap},
		"a signer revoked before": {browser.id(), append(slices.Clone(entries), add(browser, 4, tablet)), UnknownDevice},
		"another person's log":    {node.id(), entries, errPerson},
		"a fork":                  {browser.id(), []proto.Envelope{entries[0], entries[1], add(browser, 2, tablet)}, errFork},
	} {
		if l, err := Replay(c.user, c.entries); l != nil || err != c.want {
			t.Errorf("%s: got %+v, %v; want %v", what, l, err, c.want)
		}
	}
}

func TestAdopts(t *testing.T) {
	// First run: the browser writes the genesis and adds the node it paired with.
	first, err := Replay(browser.id(), []proto.Envelope{add(browser, 1, browser), add(browser, 2, node)})
	if err != nil {
		t.Fatal(err)
	}
	// Another browser: the node adds the tablet it paired with.
	grown, err := Replay(browser.id(), append(slices.Clone(first.Entries), add(node, 3, tablet)))
	if err != nil {
		t.Fatal(err)
	}
	withdrawn, err := Replay(browser.id(), append(slices.Clone(grown.Entries), revoke(browser, 4, tablet)))
	if err != nil {
		t.Fatal(err)
	}
	orphaned, err := Replay(browser.id(), append(slices.Clone(grown.Entries), revoke(browser, 4, node)))
	if err != nil {
		t.Fatal(err)
	}
	readded, err := Replay(browser.id(), append(slices.Clone(first.Entries), add(browser, 3, tablet), add(node, 4, tablet)))
	if err != nil {
		t.Fatal(err)
	}
	for what, c := range map[string]struct {
		log      *Log
		me, peer device
		want     bool
	}{
		"the node, added by the browser it paired with":        {first, node, browser, true},
		"the tablet, added by the node it paired with":         {grown, tablet, node, true},
		"the tablet, added by a device other than its peer":    {grown, tablet, browser, false},
		"a device the log does not add":                        {first, tablet, browser, false},
		"a peer that is in the log but signed nothing":         {first, node, node, false},
		"a peer that only revoked":                             {withdrawn, tablet, browser, false},
		"the tablet, added by its peer and revoked since":      {withdrawn, tablet, node, false},
		"the tablet, whose peer was revoked since":             {orphaned, tablet, node, false},
		"the tablet, added a second time by its peer":          {readded, tablet, node, true},
		"the tablet, added first by the device it paired with": {readded, tablet, browser, true},
		"an empty log": {&Log{User: browser.id()}, node, browser, false},
	} {
		if got := c.log.Adopts(c.me.id(), c.peer.id()); got != c.want {
			t.Errorf("%s: Adopts = %v, want %v", what, got, c.want)
		}
	}
}
