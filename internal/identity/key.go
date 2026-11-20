// Package identity implements contracts/envelope.md: device keys, signed
// envelopes, the device log and the pairing exchange.
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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

func verify(device, text, s string) bool {
	pub, okKey := decode(device)
	sig, okSig := decode(s)
	return okKey && okSig && len(pub) == ed25519.PublicKeySize && ed25519.Verify(pub, []byte(text), sig)
}

// decode reads a base64url value from the wire in its one canonical spelling.
// The decoder alone also takes other values of the unused trailing bits and
// skips line breaks, which would give one key several device ids.
func decode(s string) ([]byte, bool) {
	b, err := b64.DecodeString(s)
	return b, err == nil && b64.EncodeToString(b) == s
}

// isDevice reports whether id is a device id: a public key in its canonical spelling.
func isDevice(id string) bool {
	pub, ok := decode(id)
	return ok && len(pub) == ed25519.PublicKeySize
}
