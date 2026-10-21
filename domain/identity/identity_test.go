package identity_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mariotoffia/gobuzzkey/domain/attestation"
	"github.com/mariotoffia/gobuzzkey/domain/identity"
	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/relay"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// NIP-OA spec keys: owner secret 1, agent secret 2.
const (
	ownerHex = "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"
	agentHex = "c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5"
	agentSK  = "0000000000000000000000000000000000000000000000000000000000000002"
	sigHex   = "8b7df2575caf0a108374f8471722b233c53f9ff827a8b0f91861966c3b9dd5cb2e189eae9f49d72187674c2f5bd244145e10ff86c9f257ffe65a1ee5f108b369"
)

type fixture struct {
	relay relay.URL
	sk    key.SecretKey
	agent key.PublicKey
	owner key.PublicKey
	tag   attestation.AuthTag
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	u, err := relay.ParseURL("wss://relay.example")
	if err != nil {
		t.Fatal(err)
	}
	sk, err := key.ParseSecretKey(agentSK)
	if err != nil {
		t.Fatal(err)
	}
	agent, _ := key.ParsePublicKeyHex(agentHex)
	owner, _ := key.ParsePublicKeyHex(ownerHex)
	tag, err := attestation.ParseAuthTag(`["auth","` + ownerHex + `","kind=1&created_at<1713957000","` + sigHex + `"]`)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{relay: u, sk: sk, agent: agent, owner: owner, tag: tag}
}

func TestNewAttested_HoldsTheAttestation(t *testing.T) {
	f := newFixture(t)
	id, err := identity.NewAttested(f.relay, f.sk, f.agent, f.tag)
	if err != nil {
		t.Fatal(err)
	}
	att, ok := id.Attestation()
	if !ok || att.Owner() != f.owner || id.Agent() != f.agent || id.Relay() != f.relay {
		t.Fatalf("identity = %+v", id)
	}
}

func TestNewAttested_RejectsSelfAttestation(t *testing.T) {
	f := newFixture(t)
	if _, err := identity.NewAttested(f.relay, f.sk, f.owner, f.tag); !shared.IsCode(err, shared.CodeSelfAttestation) {
		t.Fatalf("want SELF_ATTESTATION, got %v", err)
	}
}

func TestNew_RequiresRelayAndKeys(t *testing.T) {
	f := newFixture(t)
	if _, err := identity.NewMember(relay.URL{}, f.sk, f.agent); !shared.IsCode(err, shared.CodeIdentityInvalid) {
		t.Fatalf("missing relay: %v", err)
	}
	if _, err := identity.NewMember(f.relay, key.SecretKey{}, f.agent); !shared.IsCode(err, shared.CodeIdentityInvalid) {
		t.Fatalf("missing secret: %v", err)
	}
	id, err := identity.NewMember(f.relay, f.sk, f.agent)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := id.Attestation(); ok {
		t.Fatal("member identity reports an attestation")
	}
}

// AgentIdentity holds the secret in an unexported field; formatting the
// identity must not reveal it under any verb.
func TestAgentIdentity_FormattingNeverRevealsTheSecret(t *testing.T) {
	f := newFixture(t)
	id, err := identity.NewAttested(f.relay, f.sk, f.agent, f.tag)
	if err != nil {
		t.Fatal(err)
	}
	b := f.sk.Bytes()
	decimal := strings.Trim(fmt.Sprint(b), "[]")
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%x", "%d", "%q"} {
		for _, val := range []any{id, &id} {
			out := fmt.Sprintf(verb, val)
			if strings.Contains(out, f.sk.Nsec()) || strings.Contains(out, agentSK) || strings.Contains(out, decimal) {
				t.Fatalf("%s revealed the secret: %s", verb, out)
			}
		}
	}
}
