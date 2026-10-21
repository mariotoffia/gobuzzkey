package key_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// NIP-19 test vectors.
const (
	nip19PubHex    = "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"
	nip19Npub      = "npub180cvv07tjdrrgpa0j7j7tmnyl2yr6yr7l8j4s3evf6u64th6gkwsyjh6w6"
	nip19SecretHex = "67dea2ed018072d675f5415ecfaed7d2597555e202d85b3d65ea4e58d2d92ffa"
	nip19Nsec      = "nsec1vl029mgpspedva04g90vltkh6fvh240zqtv9k0t9af8935ke9laqsnlfe5"
)

func TestPublicKey_NIP19VectorRoundTrips(t *testing.T) {
	fromHex, err := key.ParsePublicKey(nip19PubHex)
	if err != nil {
		t.Fatal(err)
	}
	if got := fromHex.Npub(); got != nip19Npub {
		t.Fatalf("npub = %s, want %s", got, nip19Npub)
	}
	fromNpub, err := key.ParsePublicKey(nip19Npub)
	if err != nil {
		t.Fatal(err)
	}
	if fromNpub != fromHex || fromNpub.Hex() != nip19PubHex {
		t.Fatalf("npub decoded to %s, want %s", fromNpub.Hex(), nip19PubHex)
	}
}

func TestSecretKey_NIP19VectorRoundTrips(t *testing.T) {
	fromHex, err := key.ParseSecretKey(nip19SecretHex)
	if err != nil {
		t.Fatal(err)
	}
	if got := fromHex.Nsec(); got != nip19Nsec {
		t.Fatalf("nsec = %s, want %s", got, nip19Nsec)
	}
	fromNsec, err := key.ParseSecretKey(nip19Nsec)
	if err != nil {
		t.Fatal(err)
	}
	if fromNsec.Bytes() != fromHex.Bytes() {
		t.Fatal("nsec and hex decode to different secrets")
	}
	upper, err := key.ParseSecretKey(strings.ToUpper(nip19Nsec))
	if err != nil || upper.Bytes() != fromHex.Bytes() {
		t.Fatalf("upper-case nsec: %v", err)
	}
}

func TestParse_RejectsMalformedEntities(t *testing.T) {
	flipped := nip19Npub[:len(nip19Npub)-1] + "7"
	mixed := "Npub" + nip19Npub[4:]
	cases := map[string]func() error{
		"bad checksum":        func() error { _, err := key.ParsePublicKey(flipped); return err },
		"mixed case":          func() error { _, err := key.ParsePublicKey(mixed); return err },
		"nsec as public key":  func() error { _, err := key.ParsePublicKey(nip19Nsec); return err },
		"npub as secret key":  func() error { _, err := key.ParseSecretKey(nip19Npub); return err },
		"short hex":           func() error { _, err := key.ParsePublicKey(nip19PubHex[:62]); return err },
		"non-hex":             func() error { _, err := key.ParsePublicKey(strings.Repeat("g", 64)); return err },
		"zero secret":         func() error { _, err := key.ParseSecretKey(strings.Repeat("0", 64)); return err },
		"secret equal to n":   func() error { _, err := key.ParseSecretKey(order); return err },
		"x above field prime": func() error { _, err := key.ParsePublicKey(strings.Repeat("f", 64)); return err },
	}
	for name, parse := range cases {
		t.Run(name, func(t *testing.T) {
			err := parse()
			if !shared.IsCode(err, shared.CodeInvalidKey) {
				t.Fatalf("want INVALID_KEY, got %v", err)
			}
		})
	}
}

const order = "fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141"

func TestSecretKey_AcceptsLargestScalar(t *testing.T) {
	nMinus1 := order[:63] + "0"
	if _, err := key.ParseSecretKey(nMinus1); err != nil {
		t.Fatalf("n-1 rejected: %v", err)
	}
}

func TestParsePublicKeyHex_RejectsUpperCase(t *testing.T) {
	if _, err := key.ParsePublicKeyHex(strings.ToUpper(nip19PubHex)); !shared.IsCode(err, shared.CodeInvalidKey) {
		t.Fatalf("upper-case wire hex accepted: %v", err)
	}
}

func TestPublicKey_RejectsXWithoutCurvePoint(t *testing.T) {
	// x = 5: 5^3 + 7 = 132 is a quadratic non-residue mod p.
	var b [key.Size]byte
	b[31] = 5
	if _, err := key.PublicKeyFromBytes(b); !shared.IsCode(err, shared.CodeInvalidKey) {
		t.Fatalf("x=5 accepted: %v", err)
	}
	b[31] = 1 // 1 + 7 = 8 is a residue: x=1 is on the curve
	if _, err := key.PublicKeyFromBytes(b); err != nil {
		t.Fatalf("x=1 rejected: %v", err)
	}
}

func TestSecretKey_EveryFormatVerbRedacts(t *testing.T) {
	sk, err := key.ParseSecretKey(nip19SecretHex)
	if err != nil {
		t.Fatal(err)
	}
	holder := struct{ Secret key.SecretKey }{sk}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d"} {
		for _, val := range []any{sk, holder, &holder} {
			out := fmt.Sprintf(verb, val)
			if strings.Contains(out, nip19Nsec) || strings.Contains(strings.ToLower(out), nip19SecretHex[:16]) {
				t.Fatalf("%s leaked the secret: %s", verb, out)
			}
		}
	}
}

func TestSignature_HexRoundTrip(t *testing.T) {
	s := strings.Repeat("ab", key.SignatureSize)
	sig, err := key.ParseSignatureHex(s)
	if err != nil {
		t.Fatal(err)
	}
	if sig.Hex() != s {
		t.Fatalf("round trip %s", sig.Hex())
	}
	if _, err := key.ParseSignatureHex(s[:126]); err == nil {
		t.Fatal("short signature accepted")
	}
}

// A struct field of type SecretKey must not print the secret either; fmt does
// not call Format on unexported fields.
func TestSecretKey_RedactedInsideUnexportedFields(t *testing.T) {
	sk, err := key.ParseSecretKey(nip19SecretHex)
	if err != nil {
		t.Fatal(err)
	}
	type holder struct {
		secret key.SecretKey
		nested struct{ inner key.SecretKey }
	}
	h := holder{secret: sk}
	h.nested.inner = sk
	raw := sk.Bytes()
	decimal := strings.Trim(fmt.Sprint(raw), "[]")
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%x", "%d"} {
		for _, val := range []any{h, &h} {
			out := fmt.Sprintf(verb, val)
			if strings.Contains(out, nip19Nsec) || strings.Contains(out, nip19SecretHex[:16]) ||
				strings.Contains(out, decimal[:20]) {
				t.Fatalf("%s leaked the secret through an unexported field: %s", verb, out)
			}
		}
	}
}
