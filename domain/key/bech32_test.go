package key_test

import (
	"bytes"
	"testing"

	"github.com/mariotoffia/gobuzzkey/domain/key"
)

func TestBech32_BIP173ValidStrings(t *testing.T) {
	for _, s := range []string{
		"A12UEL5L",
		"a12uel5l",
		"abcdef1qpzry9x8gf2tvdw0s3jn54khce6mua7lmqqqxw",
		"split1checkupstagehandshakeupstreamerranterredcaperred2y9e3w",
	} {
		if _, _, err := key.DecodeBech32(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
}

func TestBech32_BIP173InvalidStrings(t *testing.T) {
	for _, s := range []string{
		"pzry9x0s0muk",  // no separator
		"1pzry9x0s0muk", // empty hrp
		"x1b4n0q5v",     // invalid data character
		"li1dgmt3",      // too short checksum
		"A1G7SGD8",      // checksum computed with upper-case hrp
		"10a06t8",       // empty hrp
		"1qzzfhee",      // empty hrp
		"a12UEL5L",      // mixed case
		"abc1rzg",       // too short
		"de1lg7wt\xff",  // invalid character
		"an84characterslonghumanreadablepartthatcontainsthetheexcludedcharactersbioandnumber11d6pts4", // too long
	} {
		if _, _, err := key.DecodeBech32(s); err == nil {
			t.Errorf("%q decoded", s)
		}
	}
}

func FuzzBech32RoundTrip(f *testing.F) {
	f.Add("npub", []byte{1, 2, 3})
	f.Add("nsec", bytes.Repeat([]byte{0xff}, 32))
	f.Fuzz(func(t *testing.T, hrp string, payload []byte) {
		if hrp == "" || len(hrp) > 20 || len(payload) > 40 {
			return
		}
		for i := range len(hrp) {
			if hrp[i] < 'a' || hrp[i] > 'z' {
				return
			}
		}
		s := key.EncodeBech32(hrp, payload)
		gotHRP, got, err := key.DecodeBech32(s)
		if err != nil {
			t.Fatalf("decode %s: %v", s, err)
		}
		if gotHRP != hrp || !bytes.Equal(got, payload) {
			t.Fatalf("round trip %s -> %s %x", s, gotHRP, got)
		}
	})
}

func FuzzBech32Decode(f *testing.F) {
	f.Add("npub180cvv07tjdrrgpa0j7j7tmnyl2yr6yr7l8j4s3evf6u64th6gkwsyjh6w6")
	f.Add("a12uel5l")
	f.Fuzz(func(t *testing.T, s string) {
		hrp, payload, err := key.DecodeBech32(s)
		if err != nil {
			return
		}
		// Anything that decodes must re-encode to the lower-case input.
		if again := key.EncodeBech32(hrp, payload); again != lower(s) {
			t.Fatalf("decode/encode mismatch: %q -> %q", s, again)
		}
	})
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
