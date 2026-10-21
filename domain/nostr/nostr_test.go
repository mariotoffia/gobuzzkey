package nostr_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/nostr"
)

// The NIP-OA "Signed Event Example".
const specEvent = `{
  "id": "d892a65e7677e0554ebb70ee16deeb6a0727dba46450fb4bc001291d7bff971b",
  "pubkey": "c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5",
  "created_at": 1713956400,
  "kind": 1,
  "tags": [["auth","79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","kind=1&created_at<1713957000","8b7df2575caf0a108374f8471722b233c53f9ff827a8b0f91861966c3b9dd5cb2e189eae9f49d72187674c2f5bd244145e10ff86c9f257ffe65a1ee5f108b369"]],
  "content": "owner-attested agent event",
  "sig": "7fd38992b70b5e9e113644e51b4c8ee2227f3bdd402b1855f8786c0600394ab3ec2621742a7bad0b0000b93d4d1ae6e39525f286a3c1029f43f46c3359a6c76f"
}`

func decodeSpec(t *testing.T) nostr.Event {
	t.Helper()
	var ev nostr.Event
	if err := json.Unmarshal([]byte(specEvent), &ev); err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestComputeID_MatchesSpecSignedEvent(t *testing.T) {
	ev := decodeSpec(t)
	id := ev.ComputeID()
	if got := hex.EncodeToString(id[:]); got != ev.ID {
		t.Fatalf("id = %s, want %s", got, ev.ID)
	}
}

func TestNewUnsigned_ReproducesSpecEventID(t *testing.T) {
	ev := decodeSpec(t)
	pub, err := key.ParsePublicKeyHex(ev.PubKey)
	if err != nil {
		t.Fatal(err)
	}
	u := nostr.NewUnsigned(pub, time.Unix(ev.CreatedAt, 0), ev.Kind, ev.Tags, ev.Content)
	id := u.ID()
	if hex.EncodeToString(id[:]) != ev.ID {
		t.Fatal("unsigned event ID differs from spec")
	}
	sig, err := key.ParseSignatureHex(ev.Sig)
	if err != nil {
		t.Fatal(err)
	}
	signed := u.Signed(sig)
	if signed.ID != ev.ID || signed.Sig != ev.Sig || signed.PubKey != ev.PubKey {
		t.Fatalf("signed event %+v", signed)
	}
}

func TestSerialize_EscapesLikeSerdeJSONAndJSONStringify(t *testing.T) {
	cases := map[string]string{
		"line feed":       "a\nb",
		"quote":           `a"b`,
		"backslash":       `a\b`,
		"carriage return": "a\rb",
		"tab":             "a\tb",
		"backspace":       "a\bb",
		"form feed":       "a\fb",
	}
	want := map[string]string{
		"line feed":       `"a\nb"`,
		"quote":           `"a\"b"`,
		"backslash":       `"a\\b"`,
		"carriage return": `"a\rb"`,
		"tab":             `"a\tb"`,
		"backspace":       `"a\bb"`,
		"form feed":       `"a\fb"`,
	}
	for name, content := range cases {
		got := string(nostr.Serialize("p", 1, 1, nil, content))
		exp := `[0,"p",1,1,[],` + want[name] + `]`
		if got != exp {
			t.Errorf("%s: %s, want %s", name, got, exp)
		}
	}
	// Other C0 controls use \u00xx with lowercase hex, as serde_json and
	// JSON.stringify write them.
	for raw, want := range map[string]string{"\x00": `\u0000`, "\x01": `\u0001`, "\x1b": `\u001b`, "\x1f": `\u001f`} {
		got := string(nostr.Serialize("p", 1, 1, nil, raw))
		if exp := `[0,"p",1,1,[],"` + want + `"]`; got != exp {
			t.Errorf("%q: %s, want %s", raw, got, exp)
		}
	}
	// Characters encoding/json would escape stay verbatim.
	for _, s := range []string{"\u2028", "\u2029", "<>&", "\x7f", "é🙂", "/"} {
		got := string(nostr.Serialize("p", 1, 1, [][]string{{"t", s}}, s))
		exp := `[0,"p",1,1,[["t","` + s + `"]],"` + s + `"]`
		if got != exp {
			t.Errorf("%q escaped: %s", s, got)
		}
	}
}

func TestFilter_EncodesOnlySetFields(t *testing.T) {
	f := nostr.Filter{Kinds: []int{9}, Tags: map[string][]string{"h": {"c1"}}, Limit: 3}
	b, err := f.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"#h":["c1"],"kinds":[9],"limit":3}` {
		t.Fatalf("filter = %s", b)
	}
}

func TestHTTPAuthTags_HashesTheExactBody(t *testing.T) {
	body := []byte(`[{"kinds":[39000]}]`)
	tags := nostr.HTTPAuthTags("https://relay.example/query", "post", body, "n1")
	sum := sha256.Sum256(body)
	want := [][]string{
		{"u", "https://relay.example/query"},
		{"method", "POST"},
		{"nonce", "n1"},
		{"payload", hex.EncodeToString(sum[:])},
	}
	if len(tags) != len(want) {
		t.Fatalf("tags = %v", tags)
	}
	for i := range want {
		if tags[i][0] != want[i][0] || tags[i][1] != want[i][1] {
			t.Fatalf("tag %d = %v, want %v", i, tags[i], want[i])
		}
	}
	if got := nostr.HTTPAuthTags("u", "GET", nil, "n"); len(got) != 3 {
		t.Fatalf("payload tag added for a request without body: %v", got)
	}
}

func TestEvent_TagValueReturnsFirstMatch(t *testing.T) {
	ev := nostr.Event{Tags: [][]string{{"h"}, {"h", "first"}, {"h", "second"}}}
	if v, ok := ev.TagValue("h"); !ok || v != "first" {
		t.Fatalf("TagValue = %q %v", v, ok)
	}
	if _, ok := ev.TagValue("d"); ok {
		t.Fatal("missing tag found")
	}
}
