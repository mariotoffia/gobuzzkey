package key

import (
	"strings"

	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// BIP-173 bech32 (not bech32m), as NIP-19 uses for npub and nsec.

const bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

// bech32MaxLen is the BIP-173 limit. NIP-19 entities used here are 63 chars.
const bech32MaxLen = 90

func bech32Polymod(values []byte) uint32 {
	gen := [5]uint32{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}
	chk := uint32(1)
	for _, v := range values {
		top := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ uint32(v)
		for i := range 5 {
			if (top>>uint(i))&1 == 1 {
				chk ^= gen[i]
			}
		}
	}
	return chk
}

func bech32HRPExpand(hrp string) []byte {
	out := make([]byte, 0, len(hrp)*2+1)
	for i := range len(hrp) {
		out = append(out, hrp[i]>>5)
	}
	out = append(out, 0)
	for i := range len(hrp) {
		out = append(out, hrp[i]&31)
	}
	return out
}

func bech32Checksum(hrp string, data []byte) []byte {
	values := append(bech32HRPExpand(hrp), data...)
	values = append(values, 0, 0, 0, 0, 0, 0)
	mod := bech32Polymod(values) ^ 1
	out := make([]byte, 6)
	for i := range 6 {
		out[i] = byte(mod>>uint(5*(5-i))) & 31
	}
	return out
}

// convertBits regroups a byte slice between bit widths.
func convertBits(data []byte, from, to uint, pad bool) ([]byte, bool) {
	acc, bits := uint32(0), uint(0)
	maxv := uint32(1)<<to - 1
	out := make([]byte, 0, len(data)*int(from)/int(to)+1)
	for _, v := range data {
		if uint32(v)>>from != 0 {
			return nil, false
		}
		acc = acc<<from | uint32(v)
		bits += from
		for bits >= to {
			bits -= to
			out = append(out, byte(acc>>bits&maxv))
		}
	}
	if pad {
		if bits > 0 {
			out = append(out, byte(acc<<(to-bits)&maxv))
		}
	} else if bits >= from || acc<<(to-bits)&maxv != 0 {
		return nil, false
	}
	return out, true
}

// EncodeBech32 encodes payload bytes under hrp.
func EncodeBech32(hrp string, payload []byte) string {
	data, _ := convertBits(payload, 8, 5, true)
	combined := append(data, bech32Checksum(hrp, data)...)
	var b strings.Builder
	b.Grow(len(hrp) + 1 + len(combined))
	b.WriteString(hrp)
	b.WriteByte('1')
	for _, d := range combined {
		b.WriteByte(bech32Charset[d])
	}
	return b.String()
}

// DecodeBech32 decodes s and returns its hrp and payload bytes. Mixed case,
// a bad checksum, a bad character or non-zero padding are rejected.
func DecodeBech32(s string) (string, []byte, error) {
	if len(s) > bech32MaxLen {
		return "", nil, shared.New(shared.CodeInvalidKey, "bech32 string too long")
	}
	lower, upper := strings.ToLower(s), strings.ToUpper(s)
	if s != lower && s != upper {
		return "", nil, shared.New(shared.CodeInvalidKey, "bech32 string has mixed case")
	}
	s = lower
	sep := strings.LastIndexByte(s, '1')
	if sep < 1 || sep+7 > len(s) {
		return "", nil, shared.New(shared.CodeInvalidKey, "bech32 separator missing or misplaced")
	}
	hrp := s[:sep]
	for i := range len(hrp) {
		if hrp[i] < 33 || hrp[i] > 126 {
			return "", nil, shared.New(shared.CodeInvalidKey, "bech32 prefix has an invalid character")
		}
	}
	data := make([]byte, 0, len(s)-sep-1)
	for i := sep + 1; i < len(s); i++ {
		d := strings.IndexByte(bech32Charset, s[i])
		if d < 0 {
			return "", nil, shared.New(shared.CodeInvalidKey, "bech32 data has an invalid character")
		}
		data = append(data, byte(d))
	}
	if bech32Polymod(append(bech32HRPExpand(hrp), data...)) != 1 {
		return "", nil, shared.New(shared.CodeInvalidKey, "bech32 checksum mismatch")
	}
	payload, ok := convertBits(data[:len(data)-6], 5, 8, false)
	if !ok {
		return "", nil, shared.New(shared.CodeInvalidKey, "bech32 data has invalid padding")
	}
	return hrp, payload, nil
}
