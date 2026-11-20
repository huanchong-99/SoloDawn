package identity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

var (
	crockford    = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)
	copiedSecret = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Za-hjkmnp-tv-z]{16}$`)
)

// Secret is a pairing secret in its canonical form: 16 Crockford base32
// characters in upper case without hyphens. Its bytes are the secret bytes of
// contracts/envelope.md section 6.
type Secret string

// NewSecret returns 80 random bits as a pairing secret. How long it is valid
// and that it is used once is kept by whoever hands it out.
func NewSecret() Secret {
	b := make([]byte, 10)
	rand.Read(b)
	return Secret(crockford.EncodeToString(b))
}

// ParseSecret reads a secret as a person copied it: the hyphens are removed
// and the letters a to z made upper case, and nothing more. No other character
// is mapped to one of the alphabet, which strings.ToUpper alone would do for
// the long s.
func ParseSecret(text string) (Secret, error) {
	s := strings.ReplaceAll(text, "-", "")
	if !copiedSecret.MatchString(s) {
		return "", errors.New("not a pairing secret: want 16 characters such as 7K2M-9QXD-4F8T-WB3N")
	}
	return Secret(strings.ToUpper(s)), nil
}

// String returns the secret as shown to people: four groups of four.
func (s Secret) String() string {
	return string(s[:4] + "-" + s[4:8] + "-" + s[8:12] + "-" + s[12:])
}

// Room returns the id of the rendezvous room in which the two sides meet.
func (s Secret) Room() string {
	h := sha256.Sum256([]byte("solodawn/room/1\n" + s))
	return hex.EncodeToString(h[:16])
}

// MAC returns the mac of a pair message: it covers the role, "a" for the
// side that created the secret and "b" for the other, and the device id, kind
// and name the message carries. The text is what goes on the wire, where a
// byte that is not UTF-8 is written as U+FFFD; the other side reads the same
// bytes, a browser also for the escape of a lone surrogate.
func (s Secret) MAC(m proto.Pair) string {
	h := hmac.New(sha256.New, []byte(s))
	h.Write([]byte(string([]rune("solodawn/pair/1\n" + m.Role + "\n" + m.Key + "\n" + m.Kind + "\n" + m.Name))))
	return b64.EncodeToString(h.Sum(nil))
}

// Verify reports whether a received pair message is to be accepted: it has
// the role the other side must have, its k is a device id, its kind is node or
// browser, and its mac proves that its sender holds the secret and wrote the
// other four. A message in the receiver's own role is its own message sent
// back, and proves nothing.
func (s Secret) Verify(m proto.Pair, role string) bool {
	return m.Role == role && isDevice(m.Key) && (m.Kind == "node" || m.Kind == "browser") && hmac.Equal([]byte(m.MAC), []byte(s.MAC(m)))
}
