// Package identity implements contracts/envelope.md: device keys, signed
// envelopes, the device log and the pairing exchange.
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Each use of a device key signs under its own prefix, so a signature made
// for one purpose never verifies for the other.
const (
	envelopeContext  = "solodawn/1\n"
	challengeContext = "solodawn/rv/1\n"
)

var b64 = base64.RawURLEncoding

// Key is a device's Ed25519 private key.
type Key ed25519.PrivateKey

// LoadKey reads the device key from the node's data directory, creating it on
// first run. The file holds the 32-byte seed and is readable by the user only;
// on Windows that follows from the per-user data directory.
func LoadKey(dir string) (Key, error) {
	path := filepath.Join(dir, "device.key")
	seed, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		seed = make([]byte, ed25519.SeedSize)
		rand.Read(seed)
		err = os.WriteFile(path, seed, 0o600)
	}
	if err != nil {
		return nil, err
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("%s is not a device key: %d bytes, want %d", path, len(seed), ed25519.SeedSize)
	}
	return Key(ed25519.NewKeyFromSeed(seed)), nil
}

func (k Key) public() []byte {
	return ed25519.PrivateKey(k).Public().(ed25519.PublicKey)
}

// ID returns the device id: the public key, base64url.
func (k Key) ID() string {
	return b64.EncodeToString(k.public())
}

// Fingerprint returns the form of the device id shown to people, such as
// "3F9A 0C21 77BE".
func (k Key) Fingerprint() string {
	h := sha256.Sum256(k.public())
	return fmt.Sprintf("%X %X %X", h[0:2], h[2:4], h[4:6])
}

// Sign returns the envelope of payload p. The signature covers p as given, so
// p is the exact JSON text the receiver gets.
func (k Key) Sign(p string) proto.Envelope {
	return proto.Envelope{Payload: p, Key: k.ID(), Signature: k.sign(envelopeContext + p)}
}

// SignChallenge answers the nonce of a rendezvous handshake.
func (k Key) SignChallenge(nonce string) string {
	return k.sign(challengeContext + nonce)
}

// VerifyChallenge reports whether s is the device's answer to a rendezvous nonce.
func VerifyChallenge(device, nonce, s string) bool {
	return verify(device, challengeContext+nonce, s)
}

func (k Key) sign(text string) string {
	return b64.EncodeToString(ed25519.Sign(ed25519.PrivateKey(k), []byte(text)))
}

// verify is check 1 of contracts/envelope.md section 3: device is a device id,
// s is a 64-byte signature in its canonical spelling that does not begin with
// a weak value, and it verifies for the device over text.
func verify(device, text, s string) bool {
	pub, okKey := deviceKey(device)
	sig, okSig := decode(s)
	return okKey && okSig && len(sig) == ed25519.SignatureSize && !weak(sig[:32]) && ed25519.Verify(pub, []byte(text), sig)
}

// decode reads a base64url value from the wire in its one canonical spelling.
// The decoder alone also takes other values of the unused trailing bits and
// skips line breaks, which would give one key several device ids.
func decode(s string) ([]byte, bool) {
	b, err := b64.DecodeString(s)
	return b, err == nil && b64.EncodeToString(b) == s
}

// deviceKey returns the public key that a device id spells. A text that is
// not a device id is never used as a key.
func deviceKey(id string) ([]byte, bool) {
	pub, ok := decode(id)
	return pub, ok && len(pub) == ed25519.PublicKeySize && !weak(pub)
}

// IsDevice reports whether id is a canonical, non-weak device id.
func IsDevice(id string) bool {
	_, ok := deviceKey(id)
	return ok
}

// The encodings of the points of small order, with the highest bit of the last
// byte cleared (contracts/envelope.md section 2). Under such a key a signature
// can be made without any private key, and Ed25519 libraries differ on whether
// they verify it: crypto/ed25519 does. So these bytes are compared here, before
// the library is asked, for a public key and for the first half of a signature.
var smallOrder = map[string]bool{
	"0000000000000000000000000000000000000000000000000000000000000000": true,
	"0100000000000000000000000000000000000000000000000000000000000000": true,
	"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05": true,
	"c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a": true,
	"ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f": true,
	"edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f": true,
	"eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f": true,
}

// weak reports whether 32 bytes are one of the fourteen weak values.
func weak(point []byte) bool {
	low := [32]byte(point)
	low[31] &= 0x7f
	return smallOrder[hex.EncodeToString(low[:])]
}
