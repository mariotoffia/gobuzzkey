// Package key holds the secp256k1 key value objects Nostr uses: x-only public
// keys (hex or npub), secret keys (hex or nsec) and BIP-340 signatures. It
// validates keys with stdlib arithmetic only; signing lives behind
// ports.Signer.
package key

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// NIP-19 human-readable prefixes.
const (
	HRPPublic = "npub"
	HRPSecret = "nsec"
)

// Size is the byte length of a key.
const Size = 32

// SignatureSize is the byte length of a BIP-340 signature.
const SignatureSize = 64

func fieldPrime() *big.Int {
	p, _ := new(big.Int).SetString("fffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc2f", 16)
	return p
}

func groupOrder() *big.Int {
	n, _ := new(big.Int).SetString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 16)
	return n
}

// PublicKey is a BIP-340 x-only public key.
type PublicKey struct{ b [Size]byte }

// PublicKeyFromBytes validates b as the x coordinate of a curve point.
func PublicKeyFromBytes(b [Size]byte) (PublicKey, error) {
	x := new(big.Int).SetBytes(b[:])
	p := fieldPrime()
	if x.Cmp(p) >= 0 {
		return PublicKey{}, shared.New(shared.CodeInvalidKey, "public key is not below the field prime")
	}
	// x is valid when x^3 + 7 is a quadratic residue mod p (Euler's criterion).
	rhs := new(big.Int).Exp(x, big.NewInt(3), p)
	rhs.Add(rhs, big.NewInt(7)).Mod(rhs, p)
	exp := new(big.Int).Rsh(new(big.Int).Sub(p, big.NewInt(1)), 1)
	if new(big.Int).Exp(rhs, exp, p).Cmp(big.NewInt(1)) != 0 {
		return PublicKey{}, shared.New(shared.CodeInvalidKey, "public key is not on the secp256k1 curve")
	}
	return PublicKey{b: b}, nil
}

// ParsePublicKeyHex parses the strict wire form: 64 lowercase hex characters.
func ParsePublicKeyHex(s string) (PublicKey, error) {
	b, err := decodeLowerHex(s, Size, "public key")
	if err != nil {
		return PublicKey{}, err
	}
	var a [Size]byte
	copy(a[:], b)
	return PublicKeyFromBytes(a)
}

// ParsePublicKey parses user input: an npub, or 64 hex characters in any case.
func ParsePublicKey(s string) (PublicKey, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToLower(s), HRPPublic+"1") {
		b, err := decodeEntity(s, HRPPublic)
		if err != nil {
			return PublicKey{}, err
		}
		return PublicKeyFromBytes(b)
	}
	return ParsePublicKeyHex(strings.ToLower(s))
}

// Bytes returns the 32 key bytes.
func (k PublicKey) Bytes() [Size]byte { return k.b }

// Hex returns the lowercase hex form used on the wire.
func (k PublicKey) Hex() string { return hex.EncodeToString(k.b[:]) }

// Npub returns the NIP-19 npub form.
func (k PublicKey) Npub() string { return EncodeBech32(HRPPublic, k.b[:]) }

// IsZero reports whether k is the zero value.
func (k PublicKey) IsZero() bool { return k.b == [Size]byte{} }

// String returns the npub form.
func (k PublicKey) String() string { return k.Npub() }

// SecretKey is a secp256k1 secret scalar. Every fmt verb prints a redacted
// placeholder; use Nsec or Bytes to read the value deliberately.
//
// The bytes live inside a closure on purpose. fmt cannot call Format on an
// unexported field, so a struct holding a SecretKey would print a plain byte
// array (or even a pointer's target, under a mismatched verb such as %s). A
// func value is the one kind fmt only ever prints as an address.
type SecretKey struct{ reveal func() [Size]byte }

// SecretKeyFromBytes validates b as a scalar in [1, n-1].
func SecretKeyFromBytes(b [Size]byte) (SecretKey, error) {
	d := new(big.Int).SetBytes(b[:])
	if d.Sign() == 0 || d.Cmp(groupOrder()) >= 0 {
		return SecretKey{}, shared.New(shared.CodeInvalidKey, "secret key is outside the secp256k1 scalar range")
	}
	return SecretKey{reveal: func() [Size]byte { return b }}, nil
}

// ParseSecretKey parses an nsec, or 64 hex characters in any case.
func ParseSecretKey(s string) (SecretKey, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToLower(s), HRPSecret+"1") {
		b, err := decodeEntity(s, HRPSecret)
		if err != nil {
			return SecretKey{}, err
		}
		return SecretKeyFromBytes(b)
	}
	raw, err := decodeLowerHex(strings.ToLower(s), Size, "secret key")
	if err != nil {
		return SecretKey{}, shared.New(shared.CodeInvalidKey, "secret key must be nsec1… or 64 hex characters")
	}
	var a [Size]byte
	copy(a[:], raw)
	return SecretKeyFromBytes(a)
}

// Bytes returns a copy of the 32 secret bytes (all zero for the zero value).
func (k SecretKey) Bytes() [Size]byte {
	if k.reveal == nil {
		return [Size]byte{}
	}
	return k.reveal()
}

// Nsec returns the NIP-19 nsec form.
func (k SecretKey) Nsec() string {
	b := k.Bytes()
	return EncodeBech32(HRPSecret, b[:])
}

// IsZero reports whether k is the zero value.
func (k SecretKey) IsZero() bool { return k.reveal == nil }

const redacted = "nsec1…(redacted)"

// String returns a redacted placeholder.
func (k SecretKey) String() string { return redacted }

// GoString returns a redacted placeholder for %#v.
func (k SecretKey) GoString() string { return "key.SecretKey{" + redacted + "}" }

// Format makes every fmt verb print the redacted placeholder.
func (k SecretKey) Format(f fmt.State, verb rune) {
	if verb == 'v' && f.Flag('#') {
		_, _ = f.Write([]byte(k.GoString()))
		return
	}
	_, _ = f.Write([]byte(redacted))
}

// Signature is a 64-byte BIP-340 Schnorr signature.
type Signature [SignatureSize]byte

// ParseSignatureHex parses 128 lowercase hex characters.
func ParseSignatureHex(s string) (Signature, error) {
	b, err := decodeLowerHex(s, SignatureSize, "signature")
	if err != nil {
		return Signature{}, err
	}
	var sig Signature
	copy(sig[:], b)
	return sig, nil
}

// Hex returns the lowercase hex form.
func (s Signature) Hex() string { return hex.EncodeToString(s[:]) }

func decodeEntity(s, hrp string) ([Size]byte, error) {
	got, payload, err := DecodeBech32(s)
	if err != nil {
		return [Size]byte{}, err
	}
	if got != hrp {
		return [Size]byte{}, shared.New(shared.CodeInvalidKey, fmt.Sprintf("expected %s1…, got %s1…", hrp, got))
	}
	if len(payload) != Size {
		return [Size]byte{}, shared.New(shared.CodeInvalidKey, fmt.Sprintf("%s payload must be %d bytes", hrp, Size))
	}
	var a [Size]byte
	copy(a[:], payload)
	return a, nil
}

func decodeLowerHex(s string, n int, what string) ([]byte, error) {
	if len(s) != 2*n {
		return nil, shared.New(shared.CodeInvalidKey, fmt.Sprintf("%s must be %d hex characters", what, 2*n))
	}
	for i := range len(s) {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return nil, shared.New(shared.CodeInvalidKey, what+" must be lowercase hex")
		}
	}
	b, _ := hex.DecodeString(s)
	return b, nil
}
