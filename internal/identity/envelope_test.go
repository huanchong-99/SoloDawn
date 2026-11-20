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

func everyone(string) bool { return true }
func nobody(string) bool   { return false }

// answers opens one envelope both ways and returns the two answers as text:
// the code, or "ok" with the header.
func answers(env proto.Envelope, me string, known func(string) bool) (signal, command string) {
	text := func(h proto.PayloadHeader, err error) string {
		if err != nil {
			if h != (proto.PayloadHeader{}) {
				return fmt.Sprintf("%v with a header %+v", err, h)
			}
			return err.Error()
		}
		return fmt.Sprintf("ok %+v", h)
	}
	return text(OpenSignal(env, me, known, now)), text(OpenCommand(env, me, known, now))
}

func ok(h proto.PayloadHeader) string { return fmt.Sprintf("ok %+v", h) }

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
	// Wrong in every way a later check looks at: no JSON at all; or addressed elsewhere and old.
	junk := sender.Sign("hello")
	junkTampered := junk
	junkTampered.Payload = "hellp"
	astray := sender.Sign(payload(other, old))
	noType := sender.Sign(fmt.Sprintf(`{"n":"x","ts":%d,"to":"%s"}`, old, other))

	for name, c := range map[string]struct {
		env             proto.Envelope
		known           func(string) bool
		signal, command string
	}{
		"payload changed after signing":            {tampered, everyone, "bad_sig", "bad_sig"},
		"key of another device":                    {stolen, everyone, "bad_sig", "bad_sig"},
		"no signature":                             {unsigned, everyone, "bad_sig", "bad_sig"},
		"bad signature, unknown device, no JSON":   {junkTampered, nobody, "bad_sig", "bad_sig"},
		"unknown device":                           {good, nobody, "unknown_device", "unknown_device"},
		"unknown device, no JSON":                  {junk, nobody, "unknown_device", "unknown_device"},
		"unknown device, addressed elsewhere, old": {astray, nobody, "unknown_device", "unknown_device"},
		"no JSON":                                      {junk, everyone, "bad_request", "bad_request"},
		"no t, addressed elsewhere, old":               {noType, everyone, "bad_request", "bad_request"},
		"addressed to another device":                  {sender.Sign(payload(other, ts)), everyone, "not_for_me", "not_for_me"},
		"addressed to another device and old":          {astray, everyone, "not_for_me", "not_for_me"},
		"addressed to nobody and old":                  {sender.Sign(payload("", old)), everyone, "not_for_me", "ok {Type:post Nonce:" + testID("post") + fmt.Sprintf(" Timestamp:%d Group:", old) + testID("group") + " To:}"},
		"addressed to the receiver, three minutes old": {sender.Sign(payload(me, old)), everyone, "stale", "ok {Type:offer Nonce:" + testID("offer") + fmt.Sprintf(" Timestamp:%d Group: To:", old) + me + "}"},
	} {
		if signal, command := answers(c.env, me, c.known); signal != c.signal || command != c.command {
			t.Errorf("%s:\n as signalling %s, want %s\n as a command  %s, want %s", name, signal, c.signal, command, c.command)
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

// Check 3: t and n are strings, ts is an integer, and g and to are strings
// when they are there. Every payload here is addressed to the receiver and
// fresh, so what is not bad_request passes both ways.
func TestOpenReadsTheCommonFields(t *testing.T) {
	sender, me := testKey("browser"), testKey("node").ID()
	whole := proto.PayloadHeader{Type: "offer", Nonce: "x", Timestamp: ts, Group: "G1", To: me}
	fields := func(change func(m map[string]string)) string {
		m := map[string]string{"t": `"offer"`, "n": `"x"`, "ts": fmt.Sprint(ts), "g": `"G1"`, "to": `"` + me + `"`}
		change(m)
		var parts []string
		for _, name := range []string{"t", "n", "ts", "g", "to"} {
			if m[name] != "" {
				parts = append(parts, `"`+name+`":`+m[name])
			}
		}
		return "{" + strings.Join(parts, ",") + m["more"] + "}"
	}
	set := func(name, value string) func(map[string]string) {
		return func(m map[string]string) { m[name] = value }
	}
	without := func(h proto.PayloadHeader, change func(*proto.PayloadHeader)) string {
		change(&h)
		return ok(h)
	}
	const longS = string(rune(0x17f))

	for name, c := range map[string]struct{ p, want string }{
		"every field":                          {fields(set("", "")), ok(whole)},
		"no g":                                 {fields(set("g", "")), without(whole, func(h *proto.PayloadHeader) { h.Group = "" })},
		"an empty t and n":                     {fields(func(m map[string]string) { m["t"], m["n"] = `""`, `""` }), without(whole, func(h *proto.PayloadHeader) { h.Type, h.Nonce = "", "" })},
		"ts written with a fraction of zero":   {fields(set("ts", fmt.Sprint(ts)+".0")), ok(whole)},
		"ts written with an exponent":          {fields(set("ts", "1.7636e12")), ok(whole)},
		"ts written with digits past a double": {fields(set("ts", fmt.Sprint(ts)+".00000000000001")), ok(whole)},
		"a field the receiver does not know":   {fields(set("more", `,"conn":"c1","extra":[1,{"TS":2}]`)), ok(whole)},
		"other names in another case":          {fields(set("more", `,"T":"post","N":"y","TS":1,"G":"G2","TO":"other"`)), ok(whole)},
		"a name twice: the last counts":        {fields(set("more", `,"n":"y"`)), without(whole, func(h *proto.PayloadHeader) { h.Nonce = "y" })},

		"no t":                                  {fields(set("t", "")), "bad_request"},
		"no n":                                  {fields(set("n", "")), "bad_request"},
		"no ts":                                 {fields(set("ts", "")), "bad_request"},
		"t only in upper case":                  {fields(func(m map[string]string) { m["t"], m["more"] = "", `,"T":"offer"` }), "bad_request"},
		"ts only under a name that folds to it": {fields(func(m map[string]string) { m["ts"], m["more"] = "", `,"t`+longS+`":`+fmt.Sprint(ts) }), "bad_request"},
		"t a number":                            {fields(set("t", "1")), "bad_request"},
		"t null":                                {fields(set("t", "null")), "bad_request"},
		"n a number":                            {fields(set("n", "1")), "bad_request"},
		"n a list":                              {fields(set("n", `["x"]`)), "bad_request"},
		"ts a text":                             {fields(set("ts", `"`+fmt.Sprint(ts)+`"`)), "bad_request"},
		"ts with a fraction":                    {fields(set("ts", fmt.Sprint(ts)+".5")), "bad_request"},
		"ts null":                               {fields(set("ts", "null")), "bad_request"},
		"ts true":                               {fields(set("ts", "true")), "bad_request"},
		"ts too large for any number":           {fields(set("ts", "1e400")), "bad_request"},
		"ts one more than 2^53-1":               {fields(set("ts", "9007199254740992")), "bad_request"},
		"ts an integer written 1e20":            {fields(set("ts", "1e20")), "bad_request"},
		"an empty object for g":                 {fields(set("g", "{}")), "bad_request"},
		"g a number":                            {fields(set("g", "7")), "bad_request"},
		"g null":                                {fields(set("g", "null")), "bad_request"},
		"to a number":                           {fields(set("to", "7")), "bad_request"},
		"to null":                               {fields(set("to", "null")), "bad_request"},
		"to an object":                          {fields(set("to", `{"to":"`+me+`"}`)), "bad_request"},
		"a list":                                {"[" + fields(set("", "")) + "]", "bad_request"},
		"a text":                                {`"offer"`, "bad_request"},
		"null":                                  {"null", "bad_request"},
		"a number":                              {fmt.Sprint(ts), "bad_request"},
		"an empty object":                       {"{}", "bad_request"},
		"JSON with a tail":                      {fields(set("", "")) + "x", "bad_request"},
	} {
		if signal, command := answers(sender.Sign(c.p), me, everyone); signal != c.want || command != c.want {
			t.Errorf("%s: %s\n as signalling %s\n as a command  %s\n want          %s", name, c.p, signal, command, c.want)
		}
	}
}

// Check 4: signalling must name the receiver and a command may have no to. An
// empty to is a value like any other, and is nobody's device id.
func TestOpenToMustBeTheReceiver(t *testing.T) {
	sender, me, other := testKey("browser"), testKey("node").ID(), testKey("tablet").ID()
	p := func(to string) string { return fmt.Sprintf(`{"t":"x","n":"n1","ts":%d%s}`, ts, to) }
	h := func(to string) string { return ok(proto.PayloadHeader{Type: "x", Nonce: "n1", Timestamp: ts, To: to}) }
	for name, c := range map[string]struct{ p, signal, command string }{
		"to the receiver":                     {p(`,"to":"` + me + `"`), h(me), h(me)},
		"to another device":                   {p(`,"to":"` + other + `"`), "not_for_me", "not_for_me"},
		"to the receiver in another spelling": {p(`,"to":"` + respelled(me) + `"`), "not_for_me", "not_for_me"},
		"no to":                               {p(""), "not_for_me", h("")},
		"an empty to":                         {p(`,"to":""`), "not_for_me", "not_for_me"},
		"an empty to, TO the receiver":        {p(`,"to":"","TO":"` + me + `"`), "not_for_me", "not_for_me"},
		"to the receiver, TO another device":  {p(`,"to":"` + me + `","TO":"` + other + `"`), h(me), h(me)},
		"to another device, TO the receiver":  {p(`,"to":"` + other + `","TO":"` + me + `"`), "not_for_me", "not_for_me"},
		"only TO, the receiver":               {p(`,"TO":"` + me + `"`), "not_for_me", h("")},
		"only TO, another device":             {p(`,"TO":"` + other + `"`), "not_for_me", h("")},
		"to twice, the receiver last":         {p(`,"to":"` + other + `","to":"` + me + `"`), h(me), h(me)},
		"to twice, the receiver first":        {p(`,"to":"` + me + `","to":"` + other + `"`), "not_for_me", "not_for_me"},
	} {
		if signal, command := answers(sender.Sign(c.p), me, everyone); signal != c.signal || command != c.command {
			t.Errorf("%s:\n as signalling %s, want %s\n as a command  %s, want %s", name, signal, c.signal, command, c.command)
		}
	}
}

// Check 5: at most two minutes for signalling and 24 hours for a command,
// earlier or later; exactly that far still passes.
func TestOpenTimeWindows(t *testing.T) {
	sender, me := testKey("browser"), testKey("node").ID()
	at := func(off int64) string { return fmt.Sprint(ts + off) } // the signer's clock, off milliseconds from the receiver's
	for _, c := range []struct {
		ts              string
		signal, command bool // whether it passes
	}{
		{at(0), true, true},
		{at(120_000), true, true},
		{at(-120_000), true, true},
		{at(120_001), false, true},
		{at(-120_001), false, true},
		{at(86_400_000), false, true},
		{at(-86_400_000), false, true},
		{at(86_400_001), false, false},
		{at(-86_400_001), false, false},
		{"0", false, false},
		{"9007199254740991", false, false}, // the largest integer there is on the wire
		{"-9007199254740991", false, false},
	} {
		signal, command := answers(sender.Sign(fmt.Sprintf(`{"t":"x","n":"n1","ts":%s,"to":"%s"}`, c.ts, me)), me, everyone)
		passed := func(answer string) bool {
			if answer != "stale" && answer != "ok {Type:x Nonce:n1 Timestamp:"+c.ts+" Group: To:"+me+"}" {
				t.Errorf("ts %s: answer %s", c.ts, answer)
			}
			return answer != "stale"
		}
		if passed(signal) != c.signal || passed(command) != c.command {
			t.Errorf("ts %s: as signalling %s, as a command %s", c.ts, signal, command)
		}
	}
}

func TestCodesAreTheWireCodes(t *testing.T) {
	for code, want := range map[Code]string{
		BadSig: "bad_sig", UnknownDevice: "unknown_device", BadRequest: "bad_request", NotForMe: "not_for_me", Stale: "stale",
	} {
		if code.Error() != want {
			t.Errorf("code %q, want %q", code.Error(), want)
		}
	}
}
