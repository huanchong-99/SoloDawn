package identity

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

const ts int64 = 1763600000000

var now = time.UnixMilli(ts)

func payload(to string, at int64) string {
	if to == "" {
		return fmt.Sprintf(`{"t":"post","n":"%s","ts":%d,"g":"%s","text":"hello"}`, testID("post"), at, testID("group"))
	}
	return fmt.Sprintf(`{"t":"offer","n":"%s","ts":%d,"to":"%s","conn":"%s"}`, testID("offer"), at, to, testID("conn"))
}

var opens = map[string]func(proto.Envelope, string, func(string) bool, time.Time) (proto.PayloadHeader, error){
	"signalling": OpenSignal,
	"a command":  OpenCommand,
}

// wanted is the answer to a case of the order test: every refusal there holds
// for both kinds, except that what is stale as signalling is fresh as a command.
func wanted(kind string, code Code) error {
	if kind == "a command" && code == Stale {
		return nil
	}
	return code
}

func everyone(string) bool { return true }
func nobody(string) bool   { return false }

func TestOpenReturnsTheCommonFields(t *testing.T) {
	sender, me := testKey("browser"), testKey("node").ID()

	asked := ""
	h, err := OpenCommand(sender.Sign(payload("", ts)), me, func(device string) bool { asked = device; return true }, now)
	want := proto.PayloadHeader{Type: "post", Nonce: testID("post"), Timestamp: ts, Group: testID("group")}
	if err != nil || h != want {
		t.Errorf("payload without to: got %+v, %v; want %+v", h, err, want)
	}
	if asked != sender.ID() {
		t.Errorf("the caller was asked about %q, want the signer %q", asked, sender.ID())
	}

	h, err = OpenSignal(sender.Sign(payload(me, ts)), me, everyone, now)
	want = proto.PayloadHeader{Type: "offer", Nonce: testID("offer"), Timestamp: ts, To: me}
	if err != nil || h != want {
		t.Errorf("payload addressed to the receiver: got %+v, %v; want %+v", h, err, want)
	}
}

func TestOpenRefusesWithOneCodeInContractOrder(t *testing.T) {
	sender, me, other := testKey("browser"), testKey("node").ID(), testKey("tablet").ID()
	old := ts - 3*time.Minute.Milliseconds()

	good := sender.Sign(payload(me, ts))
	tampered := good
	tampered.Payload = strings.Replace(good.Payload, "offer", "offar", 1)
	stolen := good
	stolen.Key = other
	unsigned := good
	unsigned.Signature = ""
	// Wrong in every way a later check looks at: addressed elsewhere and old.
	astray := sender.Sign(payload(other, old))
	astrayTampered := astray
	astrayTampered.Signature = good.Signature

	for name, c := range map[string]struct {
		env   proto.Envelope
		known func(string) bool
		want  Code // of a signalling payload; three minutes are not old for a command
	}{
		"payload changed after signing":                {tampered, everyone, BadSig},
		"key of another device":                        {stolen, everyone, BadSig},
		"no signature":                                 {unsigned, everyone, BadSig},
		"bad signature from an unknown device":         {astrayTampered, nobody, BadSig},
		"unknown device":                               {good, nobody, UnknownDevice},
		"unknown device, addressed elsewhere, old":     {astray, nobody, UnknownDevice},
		"unknown device, payload not JSON":             {sender.Sign("hello"), nobody, UnknownDevice},
		"payload not JSON":                             {sender.Sign("hello"), everyone, BadRequest},
		"payload with a text where ts belongs":         {sender.Sign(`{"t":"post","n":"x","ts":"now"}`), everyone, BadRequest},
		"addressed to another device":                  {sender.Sign(payload(other, ts)), everyone, NotForMe},
		"addressed to another device and old":          {astray, everyone, NotForMe},
		"addressed to the receiver, three minutes old": {sender.Sign(payload(me, old)), everyone, Stale},
		"addressed to nobody, three minutes old":       {sender.Sign(payload("", old)), everyone, Stale},
	} {
		for kind, open := range opens {
			h, err := open(c.env, me, c.known, now)
			if want := wanted(kind, c.want); err != want || (h != proto.PayloadHeader{}) != (want == nil) {
				t.Errorf("%s, read as %s: got %+v, %v; want %v", name, kind, h, err, want)
			}
		}
	}
}

func TestOpenNeverAsksAboutAnUnverifiedKey(t *testing.T) {
	env := testKey("browser").Sign(payload("", ts))
	env.Payload += " "
	_, err := OpenSignal(env, "", func(string) bool { t.Error("known was called before the signature held"); return true }, now)
	if err != BadSig {
		t.Errorf("got %v, want bad_sig", err)
	}
}

func TestOpenTimeWindows(t *testing.T) {
	sender := testKey("browser")
	for _, c := range []struct {
		kind string
		off  int64 // the signer's clock minus the receiver's, in milliseconds
		want error
	}{
		{"signalling", 0, nil},
		{"signalling", 120_000, nil},
		{"signalling", -120_000, nil},
		{"signalling", 120_001, Stale},
		{"signalling", -120_001, Stale},
		{"a command", 0, nil},
		{"a command", 120_001, nil},
		{"a command", -120_001, nil},
		{"a command", 86_400_000, nil},
		{"a command", -86_400_000, nil},
		{"a command", 86_400_001, Stale},
		{"a command", -86_400_001, Stale},
	} {
		h, err := opens[c.kind](sender.Sign(payload("", ts+c.off)), "", everyone, now)
		if err != c.want || (err == nil) != (h.Timestamp == ts+c.off) {
			t.Errorf("%s, clock difference %d ms: got %+v, %v; want %v", c.kind, c.off, h, err, c.want)
		}
	}
}

func TestCodesAreTheWireCodes(t *testing.T) {
	for code, want := range map[Code]string{
		BadSig: "bad_sig", UnknownDevice: "unknown_device", NotForMe: "not_for_me", Stale: "stale", BadRequest: "bad_request",
	} {
		if code.Error() != want {
			t.Errorf("code %q, want %q", code.Error(), want)
		}
	}
}
