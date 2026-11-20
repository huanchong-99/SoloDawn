package identity

import (
	"encoding/hex"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// The sample secret of contracts/envelope.md section 6. Room and mac values
// were computed with another SHA-256 and HMAC implementation (Node's crypto
// module, the text encoded by its TextEncoder): macA and macB for the device
// of the RFC 8032 test key as the node "workstation", macOdd for it in role a
// as a browser whose name is odd, which holds a lone surrogate in JavaScript
// and U+FFFD here.
const (
	shown      = "7K2M-9QXD-4F8T-WB3N"
	canonical  = "7K2M9QXD4F8TWB3N"
	secretBits = "3cc544dfad23d1ae2c75"
	room       = "23b3731de722f5b7425a41416cf5d30b"
	macA       = "iWG_BRkXCtR8OKQJz6EGeQxUUffZlDp8IOm-XRifvqw"
	macB       = "dhGdvYswXbCsrMb2YuWEEhmMkUfFsrC3UA5SFI2akqI"
	macOdd     = "3VEPY8_4LjatXYx7y3qgVWyipZ2TPYHIr4mhBMe1QZc"
	odd        = "caf\u00e9\n\ufffd \U0001f642"
)

// pairOf is the pair message of a device, with the mac that follows from it.
func pairOf(s Secret, role, device, kind, name string) proto.Pair {
	m := proto.Pair{Type: "pair", Key: device, Kind: kind, Name: name, Role: role}
	m.MAC = s.MAC(m)
	return m
}

func TestParseSecretGivesTheCanonicalBytes(t *testing.T) {
	for _, text := range []string{shown, canonical, "7k2m-9qxd-4f8t-wb3n", "7k2m9qxd4f8twb3n", "7K2M9QXD-4f8tWB3N", "-7K2M9QXD4F8TWB3N-"} {
		s, err := ParseSecret(text)
		if err != nil || string(s) != canonical {
			t.Errorf("%q: got %q, %v", text, s, err)
		}
	}
	const refusal = "not a pairing secret: want 16 characters such as 7K2M-9QXD-4F8T-WB3N"
	for what, text := range map[string]string{
		"empty":               "",
		"15 characters":       "7K2M-9QXD-4F8T-WB3",
		"17 characters":       "7K2M-9QXD-4F8T-WB3NN",
		"the letter I":        "IK2M-9QXD-4F8T-WB3N",
		"the letter L":        "7K2M-9QXD-4F8T-WB3L",
		"the letter O":        "7K2M-OQXD-4F8T-WB3N",
		"the letter U":        "7K2M-9QXD-4F8U-WB3N",
		"a space for a group": "7K2M 9QXD-4F8T-WB3N",
		"spaces between":      "7K2M 9QXD 4F8T WB3N",
		"a line end":          "7K2M-9QXD-4F8T-WB3N\n",
		"a line end inside":   "7K2M-9QXD\n4F8T-WB3",
		"an underscore":       "7K2M_9QXD4F8TWB3N",
		// Characters that a general upper-casing turns into letters of the alphabet.
		"a long s for an S":     "7K2M-9QXD-4F8T-WB3" + string(rune(0x17f)),
		"a sharp s for two S":   "7K2M-9QXD-4F8T-WB" + string(rune(0xdf)),
		"a Kelvin sign for a K": "7" + string(rune(0x212a)) + "2M-9QXD-4F8T-WB3N",
		"a dotless i":           "7K2M-9QXD-4F8T-WB3" + string(rune(0x131)),
		"a full-width digit":    "7K2M-9QXD-4F8T-WB3" + string(rune(0xff17)),
	} {
		s, err := ParseSecret(text)
		if s != "" || err == nil || err.Error() != refusal {
			t.Errorf("%s: got %q, %v", what, s, err)
		}
	}
}

func TestSecretShownInFourGroups(t *testing.T) {
	if got := Secret(canonical).String(); got != shown {
		t.Errorf("shown as %q, want %q", got, shown)
	}
}

func TestNewSecretIs80RandomBitsInCrockfordBase32(t *testing.T) {
	bits, err := hex.DecodeString(secretBits)
	if err != nil {
		t.Fatal(err)
	}
	if got := crockford.EncodeToString(bits); got != canonical {
		t.Fatalf("80 bits %s encode to %q, want %q", secretBits, got, canonical)
	}
	form := regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{16}$`)
	seen := map[Secret]bool{}
	for range 64 {
		s := NewSecret()
		if !form.MatchString(string(s)) {
			t.Fatalf("%q is not 16 Crockford base32 characters", s)
		}
		if parsed, err := ParseSecret(s.String()); err != nil || parsed != s {
			t.Fatalf("%q does not read back from %q: %q, %v", s, s.String(), parsed, err)
		}
		seen[s] = true
	}
	if len(seen) != 64 {
		t.Errorf("64 secrets, %d distinct", len(seen))
	}
}

func TestRoomAndMAC(t *testing.T) {
	s := Secret(canonical)
	if got := s.Room(); got != room {
		t.Errorf("room %q, want %q", got, room)
	}
	for want, m := range map[string]proto.Pair{
		macA:   {Key: rfcDevice, Kind: "node", Name: "workstation", Role: "a"},
		macB:   {Key: rfcDevice, Kind: "node", Name: "workstation", Role: "b"},
		macOdd: {Key: rfcDevice, Kind: "browser", Name: odd, Role: "a"},
	} {
		if got := s.MAC(m); got != want {
			t.Errorf("mac of %+v: %q, want %q", m, got, want)
		}
		// The mac covers neither t nor the mac the message carries.
		m.Type, m.MAC = "pair", want
		if got := s.MAC(m); got != want {
			t.Errorf("mac of %+v: %q, want %q", m, got, want)
		}
	}
	other := Secret("7K2M9QXD4F8TWB3M")
	if got := other.Room(); got != "0f4f88cc3f19976deecf13f14accd6a9" {
		t.Errorf("room for the second secret: %q", got)
	}
	if got := pairOf(other, "a", rfcDevice, "node", "workstation").MAC; got != "LmipQTW7zfj5yC4U4DrOfzQhnJ4_OmbI8nK9u7NjIes" {
		t.Errorf("mac for the second secret: %q", got)
	}
}

// Each of the four values changes the mac, and none can pass for part of
// another: the three before name hold no line break.
func TestMACCoversRoleDeviceKindAndName(t *testing.T) {
	s := Secret(canonical)
	for _, c := range []struct{ device, kind, name, want string }{
		{node.id(), "node", "workstation", "wCzQ-sDqagt-Eafi91RHzBOweAxtxpkYIs7ZnE1Cl-M"},
		{rfcDevice, "browser", "workstation", "PP1xbPe_yYph-g2y8j-87QrZVvUr7GYky1xCzgVjRPQ"},
		{rfcDevice, "node", "workstatiom", "b9cO-OyDMSeiUG8m893sER3tllYmJ27z81PAbgawRC0"},
		{rfcDevice, "node", "", "DeqxvaMS8ErxxqSwxdRf_Q29crpSwI0yoPzJ2G1RFmo"},
		{rfcDevice, "node", "\nworkstation", "-g2M_cP32VKZvHWruxZUSIrp8MbvO9i8toA6RwCKqnE"},
		{rfcDevice, "", "node\nworkstation", "-zwt2LQ0ZUS8mlhaVTblf32_Mm0hg1Qt3qlHomwE1e4"},
		{rfcDevice, "nodeworkstation", "", "b1-rsjp92pNada8eUSSzIsXxJkTaX0Xz_zjd-lS0_L4"},
	} {
		if got := s.MAC(proto.Pair{Role: "a", Key: c.device, Kind: c.kind, Name: c.name}); got != c.want {
			t.Errorf("mac of %q %q %q: %q, want %q", c.device, c.kind, c.name, got, c.want)
		}
	}
}

// A message is written to the wire as JSON, where a byte that is not UTF-8
// becomes U+FFFD. The mac is over what the other side reads.
func TestMACIsOverTheNameAsItGoesOnTheWire(t *testing.T) {
	s := Secret(canonical)
	sent := pairOf(s, "a", rfcDevice, "node", "caf\xe9 \xff\xfe")
	if want := "F2WdtY7ibJyBGup8ldYAQ6C6HrbIk_WuqJWxk5sZLVo"; sent.MAC != want {
		t.Errorf("mac %q, want that of the name with U+FFFD for each byte, %q", sent.MAC, want)
	}
	wire, err := json.Marshal(sent)
	if err != nil {
		t.Fatal(err)
	}
	var received proto.Pair
	if err := proto.Decode(wire, &received); err != nil || !s.Verify(received, "a") {
		t.Errorf("the message read from the wire is refused: %s, %v", wire, err)
	}
	// A browser reads the escape of a lone surrogate as one, and encodes it as U+FFFD.
	if err := proto.Decode([]byte(`{"t":"pair","k":"`+rfcDevice+`","kind":"browser","name":"caf\u00e9\n\ud800 \ud83d\ude42","role":"a","mac":"`+macOdd+`"}`), &received); err != nil || !s.Verify(received, "a") {
		t.Errorf("a name with the escape of a lone surrogate is refused: %+v, %v", received, err)
	}
}

func TestVerifyPair(t *testing.T) {
	s := Secret(canonical)
	pair := func(role, device, mac string) proto.Pair {
		return proto.Pair{Type: "pair", Key: device, Kind: "node", Name: "workstation", Role: role, MAC: mac}
	}
	with := func(change func(*proto.Pair)) proto.Pair {
		m := pair("a", rfcDevice, macA)
		change(&m)
		return m
	}
	other := testKey("node").ID()
	weakKey := b64.EncodeToString(weakValues()[2])
	for what, c := range map[string]struct {
		m    proto.Pair
		role string
		want bool
	}{
		"the creator's message, read by the other side": {pair("a", rfcDevice, macA), "a", true},
		"the other side's message, read by the creator": {pair("b", rfcDevice, macB), "b", true},
		"a browser's message":                           {pairOf(s, "a", rfcDevice, "browser", "phone"), "a", true},
		"a name that is empty":                          {pairOf(s, "a", rfcDevice, "node", ""), "a", true},
		"the creator's own message sent back to it":     {pair("a", rfcDevice, macA), "b", false},
		"the other side's own message sent back to it":  {pair("b", rfcDevice, macB), "a", false},
		"the creator's mac under the other role":        {pair("b", rfcDevice, macA), "b", false},
		"the mac of another device":                     {pair("a", other, macA), "a", false},
		"the name changed on the way":                   {with(func(m *proto.Pair) { m.Name = "laptop" }), "a", false},
		"the kind changed on the way":                   {with(func(m *proto.Pair) { m.Kind = "browser" }), "a", false},
		"a kind that is neither, with its mac":          {pairOf(s, "a", rfcDevice, "phone", "workstation"), "a", false},
		"no kind, with its mac":                         {pairOf(s, "a", rfcDevice, "", "workstation"), "a", false},
		"a k that is no device id, with its mac":        {pairOf(s, "a", "workstation", "node", "workstation"), "a", false},
		"a k in another spelling, with its mac":         {pairOf(s, "a", respelled(rfcDevice), "node", "workstation"), "a", false},
		"a weak key as k, with its mac":                 {pairOf(s, "a", weakKey, "node", "workstation"), "a", false},
		"no k, with its mac":                            {pairOf(s, "a", "", "node", "workstation"), "a", false},
		"a mac made with another secret":                {pairOf(Secret("7K2M9QXD4F8TWB3M"), "a", rfcDevice, "node", "workstation"), "a", false},
		"no mac":                                        {pair("a", rfcDevice, ""), "a", false},
		"a mac cut short":                               {pair("a", rfcDevice, macA[:42]), "a", false},
	} {
		if got := s.Verify(c.m, c.role); got != c.want {
			t.Errorf("%s: Verify = %v, want %v", what, got, c.want)
		}
	}
}
