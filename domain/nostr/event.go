// Package nostr holds the slice of NIP-01 and NIP-98 gobuzzkey needs: events,
// their canonical serialization and ID, query filters and HTTP-auth tags.
package nostr

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// Event kinds used by gobuzzkey.
const (
	// KindChannelMessage is a NIP-29 group (Buzz channel) chat message.
	KindChannelMessage = 9
	// KindJoinRequest asks to join a NIP-29 group (Buzz channel).
	KindJoinRequest = 9021
	// KindLeaveRequest asks to leave a NIP-29 group (Buzz channel).
	KindLeaveRequest = 9022
	// KindHTTPAuth is a NIP-98 HTTP authorization event.
	KindHTTPAuth = 27235
	// KindChannelMetadata is NIP-29 group (Buzz channel) metadata.
	KindChannelMetadata = 39000
	// KindChannelMembers is a NIP-29 group member list, signed by the relay.
	KindChannelMembers = 39002
)

// Event is a signed Nostr event as it travels on the wire.
type Event struct {
	ID        string     `json:"id"`
	PubKey    string     `json:"pubkey"`
	CreatedAt int64      `json:"created_at"`
	Kind      int        `json:"kind"`
	Tags      [][]string `json:"tags"`
	Content   string     `json:"content"`
	Sig       string     `json:"sig"`
}

// Unsigned is an event whose ID is fixed but which carries no signature yet.
type Unsigned struct {
	pub       key.PublicKey
	createdAt int64
	kind      int
	tags      [][]string
	content   string
	id        [32]byte
}

// NewUnsigned builds an event and computes its NIP-01 ID.
func NewUnsigned(pub key.PublicKey, createdAt time.Time, kind int, tags [][]string, content string) Unsigned {
	if tags == nil {
		tags = [][]string{}
	}
	u := Unsigned{pub: pub, createdAt: createdAt.Unix(), kind: kind, tags: cloneTags(tags), content: content}
	u.id = sha256.Sum256(Serialize(u.pub.Hex(), u.createdAt, u.kind, u.tags, u.content))
	return u
}

// ID returns the event ID: SHA256 of the canonical serialization. It is also
// the BIP-340 message the author signs.
func (u Unsigned) ID() [32]byte { return u.id }

// Signed attaches sig and returns the wire event.
func (u Unsigned) Signed(sig key.Signature) Event {
	return Event{
		ID:        hex.EncodeToString(u.id[:]),
		PubKey:    u.pub.Hex(),
		CreatedAt: u.createdAt,
		Kind:      u.kind,
		Tags:      cloneTags(u.tags),
		Content:   u.content,
		Sig:       sig.Hex(),
	}
}

// ComputeID recomputes the ID of a wire event from its fields.
func (e Event) ComputeID() [32]byte {
	tags := e.Tags
	if tags == nil {
		tags = [][]string{}
	}
	return sha256.Sum256(Serialize(e.PubKey, e.CreatedAt, e.Kind, tags, e.Content))
}

// TagValue returns the second element of the first tag named name.
func (e Event) TagValue(name string) (string, bool) {
	for _, t := range e.Tags {
		if len(t) >= 2 && t[0] == name {
			return t[1], true
		}
	}
	return "", false
}

// Marshal encodes the event for transport without HTML escaping.
func (e Event) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(e); err != nil {
		return nil, shared.Wrap(shared.CodeInvalidEvent, "encode event", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Serialize returns the NIP-01 canonical form
// [0,<pubkey>,<created_at>,<kind>,<tags>,<content>] with the string escaping
// Buzz (rust-nostr, via serde_json) and nostr-tools (JSON.stringify) apply:
// " and \ are escaped, \b \t \n \f \r use their short forms, every other
// character below U+0020 becomes \u00xx (lowercase hex), and everything else,
// U+2028 and U+2029 included, is written verbatim. encoding/json is not used
// because it also escapes U+2028, U+2029 and <>&, which changes the ID.
// Strings must be valid UTF-8; callers reject anything else before signing.
func Serialize(pubHex string, createdAt int64, kind int, tags [][]string, content string) []byte {
	var b bytes.Buffer
	b.WriteString(`[0,`)
	writeString(&b, pubHex)
	b.WriteByte(',')
	b.WriteString(strconv.FormatInt(createdAt, 10))
	b.WriteByte(',')
	b.WriteString(strconv.Itoa(kind))
	b.WriteString(",[")
	for i, t := range tags {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('[')
		for j, v := range t {
			if j > 0 {
				b.WriteByte(',')
			}
			writeString(&b, v)
		}
		b.WriteByte(']')
	}
	b.WriteString("],")
	writeString(&b, content)
	b.WriteByte(']')
	return b.Bytes()
}

func writeString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for i := range len(s) {
		switch c := s[i]; c {
		case '\n':
			b.WriteString(`\n`)
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if c < 0x20 {
				const hexDigits = "0123456789abcdef"
				b.WriteString(`\u00`)
				b.WriteByte(hexDigits[c>>4])
				b.WriteByte(hexDigits[c&0xf])
				continue
			}
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
}

func cloneTags(tags [][]string) [][]string {
	out := make([][]string, len(tags))
	for i, t := range tags {
		out[i] = append([]string(nil), t...)
	}
	return out
}
