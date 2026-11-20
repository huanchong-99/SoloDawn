package identity

import (
	"encoding/hex"
	"regexp"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// The sample secret of contracts/envelope.md section 6. Room and mac values
// were computed with another SHA-256 and HMAC implementation (Node's crypto
// module), the mac for the device of the RFC 8032 test key.
const (
	shown      = "7K2M-9QXD-4F8T-WB3N"
	canonical  = "7K2M9QXD4F8TWB3N"
	secretBits = "3cc544dfad23d1ae2c75"
	room       = "23b3731de722f5b7425a41416cf5d30b"
	macA       = "zYhWxq6OswvENAXMmjHSJmtwd3T-iSMssV1V7JpZzfM"
	macB       = "F5iwkTqkIaAUSOttDmkU1DRwMDR-2J3ZIlOWNGdxBFY"
)

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
	if got := s.MAC("a", rfcDevice); got != macA {
		t.Errorf("mac of role a %q, want %q", got, macA)
	}
	if got := s.MAC("b", rfcDevice); got != macB {
		t.Errorf("mac of role b %q, want %q", got, macB)
	}
	if other := Secret("7K2M9QXD4F8TWB3M"); other.Room() == room || other.MAC("a", rfcDevice) == macA {
		t.Error("another secret gives the same room or mac")
	}
}

func TestVerifyPair(t *testing.T) {
	s := Secret(canonical)
	pair := func(role, device, mac string) proto.Pair {
		return proto.Pair{Type: "pair", Key: device, Kind: "node", Name: "workstation", Role: role, MAC: mac}
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
		"the creator's own message sent back to it":     {pair("a", rfcDevice, macA), "b", false},
		"the other side's own message sent back to it":  {pair("b", rfcDevice, macB), "a", false},
		"the creator's mac under the other role":        {pair("b", rfcDevice, macA), "b", false},
		"the mac of another device":                     {pair("a", other, macA), "a", false},
		"a k that is no device id, with its mac":        {pair("a", "workstation", s.MAC("a", "workstation")), "a", false},
		"a k in another spelling, with its mac":         {pair("a", respelled(rfcDevice), s.MAC("a", respelled(rfcDevice))), "a", false},
		"a weak key as k, with its mac":                 {pair("a", weakKey, s.MAC("a", weakKey)), "a", false},
		"no k, with its mac":                            {pair("a", "", s.MAC("a", "")), "a", false},
		"a mac made with another secret":                {pair("a", rfcDevice, Secret("7K2M9QXD4F8TWB3M").MAC("a", rfcDevice)), "a", false},
		"no mac":                                        {pair("a", rfcDevice, ""), "a", false},
		"a mac cut short":                               {pair("a", rfcDevice, macA[:42]), "a", false},
	} {
		if got := s.Verify(c.m, c.role); got != c.want {
			t.Errorf("%s: Verify = %v, want %v", what, got, c.want)
		}
	}
}
