package attestation_test

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/mariotoffia/gobuzzkey/domain/attestation"
	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// NIP-OA "Test Vectors": owner_secret = 1, agent_secret = 2.
const (
	specOwner      = "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"
	specAgent      = "c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5"
	specConditions = "kind=1&created_at<1713957000"
	specPreimage   = "nostr:agent-auth:" + specAgent + ":" + specConditions
	specMessage    = "08cdecd55af4c28d3801fd69615dcf5cc04fab3bc134b38a840bf157197069a6"
	specSig        = "8b7df2575caf0a108374f8471722b233c53f9ff827a8b0f91861966c3b9dd5cb2e189eae9f49d72187674c2f5bd244145e10ff86c9f257ffe65a1ee5f108b369"
	specTag        = `["auth","` + specOwner + `","` + specConditions + `","` + specSig + `"]`
	specTagHex     = "5b2261757468222c2237396265363637656639646362626163353561303632393563653837306230373032396266636462326463653238643935396632383135623136663831373938222c226b696e643d3126637265617465645f61743c31373133393537303030222c223862376466323537356361663061313038333734663834373137323262323333633533663966663832376138623066393138363139363663336239646435636232653138396561653966343964373231383736373463326635626432343431343565313066663836633966323537666665363561316565356631303862333639225d"
)

func pub(t *testing.T, h string) key.PublicKey {
	t.Helper()
	k, err := key.ParsePublicKeyHex(h)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func conds(t *testing.T, s string) attestation.Conditions {
	t.Helper()
	c, err := attestation.ParseConditions(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPreimageAndMessage_MatchSpecVector(t *testing.T) {
	agent := pub(t, specAgent)
	c := conds(t, specConditions)
	if got := string(attestation.Preimage(agent, c)); got != specPreimage {
		t.Fatalf("preimage = %s", got)
	}
	msg := attestation.Message(agent, c)
	if got := hex.EncodeToString(msg[:]); got != specMessage {
		t.Fatalf("message = %s, want %s", got, specMessage)
	}
}

func TestAuthTag_SpecTagRoundTripsByteForByte(t *testing.T) {
	tag, err := attestation.ParseAuthTag(specTag)
	if err != nil {
		t.Fatal(err)
	}
	if tag.Owner.Hex() != specOwner || tag.Conditions.String() != specConditions || tag.Sig.Hex() != specSig {
		t.Fatalf("decoded %+v", tag)
	}
	if got := tag.String(); got != specTag {
		t.Fatalf("encoded %s", got)
	}
	if got := hex.EncodeToString([]byte(tag.String())); got != specTagHex {
		t.Fatal("tag bytes differ from the NIP-OA tag-bytes-hex vector")
	}
}

func TestAuthTag_RejectsMalformed(t *testing.T) {
	sigs := `"` + specSig + `"`
	cases := map[string]string{
		"three elements":   `["auth","` + specOwner + `",` + sigs + `]`,
		"five elements":    `["auth","` + specOwner + `","",` + sigs + `,"x"]`,
		"wrong name":       `["delegation","` + specOwner + `","",` + sigs + `]`,
		"upper-case owner": `["auth","` + strings.ToUpper(specOwner) + `","",` + sigs + `]`,
		"short signature":  `["auth","` + specOwner + `","","abcd"]`,
		"trailing &":       `["auth","` + specOwner + `","kind=1&",` + sigs + `]`,
		"leading zero":     `["auth","` + specOwner + `","kind=01",` + sigs + `]`,
		"not an array":     `{"auth":true}`,
		"number element":   `["auth","` + specOwner + `",1,` + sigs + `]`,
		"null conditions":  `["auth","` + specOwner + `",null,` + sigs + `]`,
		"null name":        `[null,"` + specOwner + `","",` + sigs + `]`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := attestation.ParseAuthTag(in); !shared.IsCode(err, shared.CodeInvalidTag) {
				t.Fatalf("want INVALID_TAG, got %v", err)
			}
		})
	}
}

func TestConditions_Grammar(t *testing.T) {
	valid := []string{
		"",
		"kind=0",
		"kind=65535",
		"created_at<4294967295",
		"created_at>0",
		specConditions,
		"created_at>1&created_at<2&kind=9",
	}
	for _, s := range valid {
		c, err := attestation.ParseConditions(s)
		if err != nil {
			t.Errorf("%q rejected: %v", s, err)
			continue
		}
		if c.String() != s {
			t.Errorf("%q not kept verbatim: %q", s, c.String())
		}
	}
	invalid := []string{
		"kind=1&",               // trailing delimiter (spec invalid vector)
		"kind=01",               // leading zero (spec invalid vector)
		"&kind=1",               // leading delimiter
		"kind=1&&kind=2",        // empty clause
		"kind=65536",            // out of range
		"created_at<4294967296", // out of range
		"kind = 1",              // whitespace
		"kind=1 ",               // trailing whitespace
		"kind<1",                // unsupported operator for kind
		"created_at=1",          // unsupported operator for created_at
		"expires<1",             // unknown clause
		"Kind=1",                // case-sensitive
		"kind=",                 // missing number
		"kind=-1",               // sign
		"kind=+1",               // sign
		"kind=1e3",              // not decimal
	}
	for _, s := range invalid {
		if _, err := attestation.ParseConditions(s); !shared.IsCode(err, shared.CodeInvalidConditions) {
			t.Errorf("%q accepted or wrong code: %v", s, err)
		}
	}
}

func TestConditions_AllowsEvaluatesEveryClause(t *testing.T) {
	c := conds(t, specConditions)
	if !c.Allows(1, 1713956400) {
		t.Fatal("spec example event not allowed")
	}
	if c.Allows(9, 1713956400) {
		t.Fatal("kind 9 allowed under kind=1")
	}
	if c.Allows(1, 1713957000) {
		t.Fatal("created_at equal to the < bound allowed")
	}
	if !c.AllowsAdmission(1713956999) {
		t.Fatal("admission must ignore kind clauses")
	}
	if c.AllowsAdmission(1713957000) {
		t.Fatal("admission must honour created_at<")
	}
	after := conds(t, "created_at>100")
	if after.AllowsAdmission(100) || !after.AllowsAdmission(101) {
		t.Fatal("created_at> is strict")
	}
}

func TestConditions_ExpiryIsEarliestUpperBound(t *testing.T) {
	c := conds(t, "created_at<300&created_at>1&created_at<200")
	exp, ok := c.Expiry()
	if !ok || exp.Unix() != 200 {
		t.Fatalf("expiry = %v %v", exp, ok)
	}
	if _, ok := conds(t, "kind=1").Expiry(); ok {
		t.Fatal("expiry without created_at<")
	}
}

func TestBuild_UntilProducesCanonicalClause(t *testing.T) {
	c, err := attestation.Build(attestation.Kind(9), attestation.Until(time.Unix(1800000000, 0)))
	if err != nil {
		t.Fatal(err)
	}
	if c.String() != "kind=9&created_at<1800000000" {
		t.Fatalf("built %q", c.String())
	}
}

func TestAttestation_RejectsSelfAttestation(t *testing.T) {
	tag, err := attestation.ParseAuthTag(specTag)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := attestation.New(tag.Owner, tag); !shared.IsCode(err, shared.CodeSelfAttestation) {
		t.Fatalf("want SELF_ATTESTATION, got %v", err)
	}
	att, err := attestation.New(pub(t, specAgent), tag)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(func() []byte { m := att.Message(); return m[:] }()) != specMessage {
		t.Fatal("attestation message differs from spec")
	}
	if att.AdmittedAt(time.Unix(1713957000, 0)) {
		t.Fatal("admitted at the expiry second")
	}
}

func FuzzParseConditions(f *testing.F) {
	for _, s := range []string{"", "kind=1", specConditions, "kind=1&", "created_at>0&kind=65535"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		c, err := attestation.ParseConditions(s)
		if err != nil {
			return
		}
		if c.String() != s {
			t.Fatalf("accepted %q but kept %q", s, c.String())
		}
		rebuilt, err := attestation.Build(c.Clauses()...)
		if err != nil || rebuilt.String() != s {
			t.Fatalf("clauses of %q rebuild to %q (%v)", s, rebuilt.String(), err)
		}
	})
}

func FuzzParseAuthTag(f *testing.F) {
	f.Add(specTag)
	f.Add(`["auth","","",""]`)
	f.Fuzz(func(t *testing.T, s string) {
		tag, err := attestation.ParseAuthTag(s)
		if err != nil {
			return
		}
		again, err := attestation.ParseAuthTag(tag.String())
		if err != nil || again.String() != tag.String() {
			t.Fatalf("re-encoding %q does not round-trip: %v", s, err)
		}
	})
}
