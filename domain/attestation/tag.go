package attestation

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"strconv"

	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// DomainSeparator prefixes every NIP-OA signing preimage.
const DomainSeparator = "nostr:agent-auth:"

// TagName is the first element of the tag array.
const TagName = "auth"

// Preimage returns the exact bytes the owner signs a hash of.
func Preimage(agent key.PublicKey, c Conditions) []byte {
	return []byte(DomainSeparator + agent.Hex() + ":" + c.String())
}

// Message returns SHA256(Preimage(agent, c)), the BIP-340 message.
func Message(agent key.PublicKey, c Conditions) [32]byte {
	return sha256.Sum256(Preimage(agent, c))
}

// AuthTag is the NIP-OA credential ["auth", owner, conditions, sig].
//
// value-object
type AuthTag struct {
	Owner      key.PublicKey
	Conditions Conditions
	Sig        key.Signature
}

// ParseAuthTag decodes the JSON array form and validates every element:
// exactly four strings, name "auth", a lowercase-hex owner key on the curve,
// a grammatical conditions string and a lowercase-hex 64-byte signature.
func ParseAuthTag(s string) (AuthTag, error) {
	var t AuthTag
	if err := t.UnmarshalJSON([]byte(s)); err != nil {
		return AuthTag{}, err
	}
	return t, nil
}

// MarshalJSON encodes the tag as the 4-element array. HTML characters are not
// escaped: the tag is sent verbatim in the x-auth-tag header and in events.
func (t AuthTag) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode([]string{TagName, t.Owner.Hex(), t.Conditions.String(), t.Sig.Hex()}); err != nil {
		return nil, shared.Wrap(shared.CodeInvalidTag, "encode auth tag", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// UnmarshalJSON decodes and validates the 4-element array.
func (t *AuthTag) UnmarshalJSON(b []byte) error {
	// Decode element by element: unmarshalling straight into []string would
	// turn a JSON null into "", which Buzz rejects.
	var raw []json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(b), &raw); err != nil {
		return shared.Wrap(shared.CodeInvalidTag, "auth tag must be a JSON array of strings", err)
	}
	if len(raw) != 4 {
		return shared.New(shared.CodeInvalidTag, "auth tag must have exactly 4 elements, got "+strconv.Itoa(len(raw)))
	}
	parts := make([]string, len(raw))
	for i, r := range raw {
		r = bytes.TrimSpace(r)
		if len(r) == 0 || r[0] != '"' || json.Unmarshal(r, &parts[i]) != nil {
			return shared.New(shared.CodeInvalidTag, "auth tag element "+strconv.Itoa(i)+" must be a JSON string")
		}
	}
	if parts[0] != TagName {
		return shared.New(shared.CodeInvalidTag, "auth tag name must be \"auth\", got "+strconv.Quote(parts[0]))
	}
	owner, err := key.ParsePublicKeyHex(parts[1])
	if err != nil {
		return shared.Wrap(shared.CodeInvalidTag, "auth tag owner", err)
	}
	conds, err := ParseConditions(parts[2])
	if err != nil {
		return shared.Wrap(shared.CodeInvalidTag, "auth tag conditions", err)
	}
	sig, err := key.ParseSignatureHex(parts[3])
	if err != nil {
		return shared.Wrap(shared.CodeInvalidTag, "auth tag signature", err)
	}
	*t = AuthTag{Owner: owner, Conditions: conds, Sig: sig}
	return nil
}

// Strings returns the tag as the 4 strings a Nostr event carries.
func (t AuthTag) Strings() []string {
	return []string{TagName, t.Owner.Hex(), t.Conditions.String(), t.Sig.Hex()}
}

// String returns the JSON form, as BUZZ_AUTH_TAG and x-auth-tag carry it.
func (t AuthTag) String() string {
	b, err := t.MarshalJSON()
	if err != nil {
		return ""
	}
	return string(b)
}
