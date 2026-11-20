package identity

import (
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"testing"
)

// What an attacker, or a careless signer, can put on the wire without the
// library's help. The arithmetic is done here with math/big, so these tests
// do not ask the code under test what a weak value is.

var (
	fieldPrime = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))
	groupOrder = mustInt("7237005577332262213973186563042994240857116359379907606001950938285454250989") // 2^252 + 27742317777372353535851937790883648493
)

func mustInt(decimal string) *big.Int {
	n, ok := new(big.Int).SetString(decimal, 10)
	if !ok {
		panic("not a number: " + decimal)
	}
	return n
}

func littleEndian(n *big.Int) []byte {
	b := n.FillBytes(make([]byte, 32))
	slices.Reverse(b)
	return b
}

func fromLittleEndian(b []byte) *big.Int {
	b = slices.Clone(b)
	slices.Reverse(b)
	return new(big.Int).SetBytes(b)
}

// weakValues returns the fourteen values of contracts/envelope.md section 2:
// the seven listed there, each with the highest bit of the last byte clear and set.
func weakValues() [][]byte {
	var out [][]byte
	for _, h := range []string{
		"0000000000000000000000000000000000000000000000000000000000000000",
		"0100000000000000000000000000000000000000000000000000000000000000",
		"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05",
		"c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a",
		"ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
		"edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
		"eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
	} {
		low, err := hex.DecodeString(h)
		if err != nil {
			panic(err)
		}
		high := slices.Clone(low)
		high[31] |= 0x80
		out = append(out, low, high)
	}
	return out
}

// forgery returns a payload and a signature over it that crypto/ed25519
// verifies under a weak public key, made without any private key: the
// signature is a small-order point and a zero scalar. Whether it verifies
// depends on the text, so the payload carries a counter that is tried upwards.
func forgery(weakKey []byte) (p string, sig []byte) {
	for i := range 4096 {
		p = fmt.Sprintf(`{"t":"post","n":"forged-%d","ts":%d,"text":"signed by nobody"}`, i, ts)
		for _, r := range weakValues() {
			sig = append(slices.Clone(r), make([]byte, 32)...)
			if ed25519.Verify(weakKey, []byte(envelopeContext+p), sig) {
				return p, sig
			}
		}
	}
	panic("no forgery found under " + hex.EncodeToString(weakKey))
}

// identitySignature signs text with a real key, but with the identity point in
// place of the random one: R is 01 00 … 00 and S is the challenge times the
// private scalar. RFC 8032 verification accepts it; a library that refuses
// small-order points does not.
func identitySignature(k Key, text string) []byte {
	r := weakValues()[2]
	h := sha512.Sum512(ed25519.PrivateKey(k).Seed())
	h[0] &= 248
	h[31] &= 127
	h[31] |= 64
	secret := fromLittleEndian(h[:32])
	digest := sha512.Sum512(slices.Concat(r, k.public(), []byte(text)))
	challenge := new(big.Int).Mod(fromLittleEndian(digest[:]), groupOrder)
	s := new(big.Int).Mod(new(big.Int).Mul(challenge, secret), groupOrder)
	return append(slices.Clone(r), littleEndian(s)...)
}

// plusOrder returns the signature with the group order added to its scalar:
// the same scalar for the arithmetic, in bytes no signer writes.
func plusOrder(sig []byte) []byte {
	s := new(big.Int).Add(fromLittleEndian(sig[32:]), groupOrder)
	return append(slices.Clone(sig[:32]), littleEndian(s)...)
}

// offCurve returns 32 bytes that are not the encoding of any point: the
// smallest y for which the curve equation has no x.
func offCurve() []byte {
	d := new(big.Int).Mul(big.NewInt(-121665), new(big.Int).ModInverse(big.NewInt(121666), fieldPrime))
	for y := int64(2); ; y++ {
		yy := big.NewInt(y * y)
		u := new(big.Int).Sub(yy, big.NewInt(1))
		v := new(big.Int).Add(new(big.Int).Mul(d, yy), big.NewInt(1))
		xx := new(big.Int).Mod(new(big.Int).Mul(u, new(big.Int).ModInverse(new(big.Int).Mod(v, fieldPrime), fieldPrime)), fieldPrime)
		if new(big.Int).ModSqrt(xx, fieldPrime) == nil {
			return littleEndian(big.NewInt(y))
		}
	}
}

func TestWeakKeysAreRefusedBeforeTheLibraryIsAsked(t *testing.T) {
	weak := weakValues()
	if len(weak) != 14 {
		t.Fatalf("%d weak values", len(weak))
	}
	for _, key := range weak {
		id := b64.EncodeToString(key)
		p, sig := forgery(key)
		if !ed25519.Verify(key, []byte(envelopeContext+p), sig) {
			t.Fatalf("%x: the forgery does not verify in crypto/ed25519", key)
		}
		if verify(id, envelopeContext+p, b64.EncodeToString(sig)) || IsDevice(id) {
			t.Errorf("%x is taken for a device id", key)
		}
		// Under a weak key even a signature that begins with an ordinary point is refused.
		honest := testKey("browser").sign(envelopeContext + p)
		if verify(id, envelopeContext+p, honest) {
			t.Errorf("%x verifies a signature", key)
		}
	}
	for _, k := range []Key{testKey("browser"), testKey("node"), rfcKey(t)} {
		if !IsDevice(k.ID()) {
			t.Errorf("%s is not taken for a device id", k.ID())
		}
	}
}

func TestSignatureThatBeginsWithAWeakValueIsRefused(t *testing.T) {
	k := testKey("browser")
	text := envelopeContext + payload("", ts)
	sig := identitySignature(k, text)
	if !ed25519.Verify(k.public(), []byte(text), sig) {
		t.Fatal("the signature that begins with the identity point does not verify in crypto/ed25519")
	}
	if verify(k.ID(), text, b64.EncodeToString(sig)) {
		t.Error("a signature that begins with the identity point verifies")
	}
	// Each of the fourteen values in the first half, whatever follows.
	good, _ := b64.DecodeString(k.sign(text))
	for _, r := range weakValues() {
		if verify(k.ID(), text, b64.EncodeToString(append(slices.Clone(r), good[32:]...))) {
			t.Errorf("a signature that begins with %x verifies", r)
		}
	}
	if !verify(k.ID(), text, b64.EncodeToString(good)) {
		t.Error("the honest signature does not verify")
	}
}

func TestSignatureScalarMustBeBelowTheGroupOrder(t *testing.T) {
	k := testKey("browser")
	text := envelopeContext + payload("", ts)
	good, _ := b64.DecodeString(k.sign(text))
	high := plusOrder(good)
	if string(high[:32]) != string(good[:32]) || string(high[32:]) == string(good[32:]) || len(high) != 64 {
		t.Fatalf("plusOrder gave %x for %x", high, good)
	}
	if ed25519.Verify(k.public(), []byte(text), high) || verify(k.ID(), text, b64.EncodeToString(high)) {
		t.Error("a signature whose scalar is not below the group order verifies")
	}
}

func TestKeyThatIsNoPointVerifiesNothing(t *testing.T) {
	key := offCurve()
	if got := hex.EncodeToString(key); !strings.HasSuffix(got, strings.Repeat("00", 31)) || key[0] < 2 {
		t.Fatalf("offCurve gave %s", got)
	}
	text := envelopeContext + payload("", ts)
	if verify(b64.EncodeToString(key), text, testKey("browser").sign(text)) {
		t.Error("32 bytes that are no point verify a signature")
	}
}

func TestSignatureMustBe64Bytes(t *testing.T) {
	k := testKey("browser")
	text := envelopeContext + payload("", ts)
	good, _ := b64.DecodeString(k.sign(text))
	for _, n := range []int{0, 1, 31, 32, 33, 63, 65, 96} {
		sig := append(slices.Clone(good), make([]byte, 32)...)[:n]
		if verify(k.ID(), text, b64.EncodeToString(sig)) {
			t.Errorf("a signature of %d bytes verifies", n)
		}
	}
}
