package identity

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Reproducible keys, never application keys. "browser" and "node" are the
// signers of the wire examples in internal/proto.
func testKey(name string) Key {
	seed := sha256.Sum256([]byte("SoloDawn protocol example: " + name))
	return Key(ed25519.NewKeyFromSeed(seed[:]))
}

func testID(label string) string {
	h := sha256.Sum256([]byte(label))
	return b64.EncodeToString(h[:16])
}

// RFC 8032 section 7.1, test 1. The values derived from it below were computed
// with another Ed25519 and SHA-256 implementation (Node's crypto module).
const (
	rfcSeed        = "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60"
	rfcDevice      = "11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo"
	rfcFingerprint = "21FE 31DF A154"
	rfcPayload     = `{"t":"post","n":"Zk3","ts":1763600000000,"text":"hello"}`
	rfcSignature   = "h9rHW69cOUSrAm2XoVQGhw-KDTZ8UoGHAd4JSwSKbr5HCbcbjyksQ0WDu0H1U4msxYodQfg-HtLxgOEJhQa7Ag"
	rfcChallenge   = "e4ng9SRF_MG-lDZek0Z2nPC87eNDLcptdckxZHYl7O4BPzWODfK2LVQQ2aTk8QeRaHxouMJpVbsV1gnSQRv3DQ"
)

func rfcKey(t *testing.T) Key {
	t.Helper()
	seed, err := hex.DecodeString(rfcSeed)
	if err != nil {
		t.Fatal(err)
	}
	return Key(ed25519.NewKeyFromSeed(seed))
}

func TestLoadKeyCreatesTheKeyOnceAndKeepsIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "device.key")

	first, err := LoadKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(seed) != 32 || first.ID() != Key(ed25519.NewKeyFromSeed(seed)).ID() {
		t.Fatalf("device.key holds %d bytes that do not give device %s", len(seed), first.ID())
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows has no owner-only mode bits; there the per-user data directory protects the file.
	if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm != 0o600 {
		t.Errorf("device.key has mode %o, want 600", perm)
	}

	second, err := LoadKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID() != first.ID() {
		t.Errorf("second load gave device %s, first %s", second.ID(), first.ID())
	}

	other, err := LoadKey(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if other.ID() == first.ID() {
		t.Error("two installations got the same device key")
	}
}

func TestLoadKeyReadsAnExistingSeed(t *testing.T) {
	dir := t.TempDir()
	seed, _ := hex.DecodeString(rfcSeed)
	if err := os.WriteFile(filepath.Join(dir, "device.key"), seed, 0o600); err != nil {
		t.Fatal(err)
	}
	k, err := LoadKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	if k.ID() != rfcDevice || k.Fingerprint() != rfcFingerprint {
		t.Errorf("got device %s, fingerprint %s", k.ID(), k.Fingerprint())
	}
}

func TestLoadKeyRefusesAFileThatIsNotASeed(t *testing.T) {
	for _, size := range []int{0, 31, 33, 64} {
		dir := t.TempDir()
		path := filepath.Join(dir, "device.key")
		if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
		k, err := LoadKey(dir)
		want := fmt.Sprintf("%s is not a device key: %d bytes, want 32", path, size)
		if k != nil || err == nil || err.Error() != want {
			t.Errorf("%d bytes: got key %v, error %v; want error %q", size, k, err, want)
		}
	}
}

func TestLoadKeyReportsFileErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	if k, err := LoadKey(missing); k != nil || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing data directory: got key %v, error %v", k, err)
	}
	if _, err := os.Stat(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("LoadKey created the data directory: %v", err)
	}

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "device.key"), 0o700); err != nil {
		t.Fatal(err)
	}
	if k, err := LoadKey(dir); k != nil || err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("unreadable key file: got key %v, error %v", k, err)
	}
}

func TestDeviceIDAndFingerprint(t *testing.T) {
	k := rfcKey(t)
	if got := k.ID(); got != rfcDevice || len(got) != 43 {
		t.Errorf("device id %q, want %q", got, rfcDevice)
	}
	if got := k.Fingerprint(); got != rfcFingerprint {
		t.Errorf("fingerprint %q, want %q", got, rfcFingerprint)
	}
}

func TestSignCoversThePrefixAndThePayloadAsGiven(t *testing.T) {
	k := rfcKey(t)
	env := k.Sign(rfcPayload)
	if env.Payload != rfcPayload || env.Key != rfcDevice || env.Signature != rfcSignature {
		t.Fatalf("got %+v", env)
	}
	// The same JSON value in other bytes is another payload.
	spaced := k.Sign(" " + rfcPayload)
	if spaced.Payload != " "+rfcPayload || spaced.Signature == rfcSignature {
		t.Errorf("a payload with a leading space got %+v", spaced)
	}
	if !verify(rfcDevice, envelopeContext+" "+rfcPayload, spaced.Signature) {
		t.Error("the signature of the spaced payload does not verify over its own bytes")
	}
}

func TestVerify(t *testing.T) {
	text := envelopeContext + rfcPayload
	other := testKey("node")
	short := b64.EncodeToString(make([]byte, 31))
	long := b64.EncodeToString(make([]byte, 33))
	for name, c := range map[string]struct {
		device, text, s string
		want            bool
	}{
		"valid":                       {rfcDevice, text, rfcSignature, true},
		"one byte of the text off":    {rfcDevice, strings.Replace(text, "hello", "hellp", 1), rfcSignature, false},
		"without the prefix":          {rfcDevice, rfcPayload, rfcSignature, false},
		"another device":              {other.ID(), text, rfcSignature, false},
		"signature of another device": {rfcDevice, text, other.sign(text), false},
		"key that is not base64url":   {rfcDevice + "!", text, rfcSignature, false},
		"key with padding":            {rfcDevice + "=", text, rfcSignature, false},
		"key of 31 bytes":             {short, text, rfcSignature, false},
		"key of 33 bytes":             {long, text, rfcSignature, false},
		"empty key":                   {"", text, rfcSignature, false},
		"signature not base64url":     {rfcDevice, text, rfcSignature + "!", false},
		"signature cut short":         {rfcDevice, text, rfcSignature[:80], false},
		"empty signature":             {rfcDevice, text, "", false},
	} {
		if got := verify(c.device, c.text, c.s); got != c.want {
			t.Errorf("%s: verify = %v, want %v", name, got, c.want)
		}
	}
}

func TestRendezvousChallenge(t *testing.T) {
	k := rfcKey(t)
	if got := k.SignChallenge("q0J"); got != rfcChallenge {
		t.Fatalf("answer %q, want %q", got, rfcChallenge)
	}
	for name, c := range map[string]struct {
		device, nonce, s string
		want             bool
	}{
		"valid":                    {rfcDevice, "q0J", rfcChallenge, true},
		"another nonce":            {rfcDevice, "q0K", rfcChallenge, false},
		"another device":           {testKey("node").ID(), "q0J", rfcChallenge, false},
		"an envelope signature":    {rfcDevice, rfcPayload, rfcSignature, false},
		"signed as an envelope":    {rfcDevice, "q0J", k.Sign("q0J").Signature, false},
		"a key that is not a key":  {"q0J", "q0J", rfcChallenge, false},
		"answer that is not bytes": {rfcDevice, "q0J", "?", false},
	} {
		if got := VerifyChallenge(c.device, c.nonce, c.s); got != c.want {
			t.Errorf("%s: VerifyChallenge = %v, want %v", name, got, c.want)
		}
	}
	// The answer to a challenge is never a valid envelope signature.
	if verify(rfcDevice, envelopeContext+"q0J", rfcChallenge) {
		t.Error("a challenge answer verifies as an envelope")
	}
}
