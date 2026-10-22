package app

import (
	"context"
	"strings"
	"time"

	"github.com/mariotoffia/gobuzzkey/domain/attestation"
	"github.com/mariotoffia/gobuzzkey/domain/clock"
	"github.com/mariotoffia/gobuzzkey/domain/identity"
	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/relay"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
	"github.com/mariotoffia/gobuzzkey/ports"
)

// Inspector opens stored identities and verifies attestations.
type Inspector struct {
	signer   ports.Signer
	verifier ports.Verifier
	clock    clock.Clock
}

// NewInspector wires the inspect use case.
func NewInspector(signer ports.Signer, verifier ports.Verifier, clk clock.Clock) *Inspector {
	return &Inspector{signer: signer, verifier: verifier, clock: clk}
}

// Report describes an attestation as a relay would judge it now.
type Report struct {
	Agent      key.PublicKey
	Attested   bool
	Owner      key.PublicKey
	Conditions string
	Expiry     time.Time // zero when the attestation never expires
	Expired    bool      // the relay would refuse it now
	Relay      relay.URL // zero when verifying a bare tag
}

// Open reads a stored identity, checks that the secret key matches the
// attested agent key and that the owner signature verifies.
func (i *Inspector) Open(ctx context.Context, src ports.IdentitySource) (identity.AgentIdentity, error) {
	rec, err := src.Load(ctx)
	if err != nil {
		return identity.AgentIdentity{}, err
	}
	relayURL, err := relay.ParseURL(rec.RelayURL)
	if err != nil {
		return identity.AgentIdentity{}, shared.Wrap(shared.CodeIdentityInvalid, "BUZZ_RELAY_URL", err)
	}
	secret, err := key.ParseSecretKey(rec.PrivateKey)
	if err != nil {
		return identity.AgentIdentity{}, shared.Wrap(shared.CodeIdentityInvalid, "BUZZ_PRIVATE_KEY", err)
	}
	agent, err := i.signer.PublicKey(secret)
	if err != nil {
		return identity.AgentIdentity{}, err
	}
	if strings.TrimSpace(rec.AuthTag) == "" {
		return identity.NewMember(relayURL, secret, agent)
	}
	tag, err := attestation.ParseAuthTag(rec.AuthTag)
	if err != nil {
		return identity.AgentIdentity{}, shared.Wrap(shared.CodeIdentityInvalid, "BUZZ_AUTH_TAG", err)
	}
	if err := i.verify(agent, tag); err != nil {
		return identity.AgentIdentity{}, err
	}
	return identity.NewAttested(relayURL, secret, agent, tag)
}

// Reusable reports whether an opened identity can serve relayURL now: it is
// for the same relay and its attestation has not expired. An identity for
// another relay is IDENTITY_EXISTS, because a new one would replace it.
func (i *Inspector) Reusable(id identity.AgentIdentity, relayURL string) error {
	want, err := relay.ParseURL(relayURL)
	if err != nil {
		return err
	}
	if !strings.EqualFold(want.String(), id.Relay().String()) {
		return shared.New(shared.CodeIdentityExists,
			"the identity is for relay "+id.Relay().String()+", not "+want.String())
	}
	if att, ok := id.Attestation(); ok && !att.AdmittedAt(i.clock.Now()) {
		exp, _ := att.Expiry()
		return shared.New(shared.CodeAttestationExpired,
			"the identity's attestation expired at "+exp.Format(time.RFC3339))
	}
	return nil
}

// Inspect reports on an opened identity.
func (i *Inspector) Inspect(id identity.AgentIdentity) Report {
	r := Report{Agent: id.Agent(), Relay: id.Relay()}
	if att, ok := id.Attestation(); ok {
		i.describe(&r, att)
	}
	return r
}

// VerifyTag verifies a bare auth tag for an agent given as npub or hex,
// without any secret key.
func (i *Inspector) VerifyTag(tagJSON, agentRef string) (Report, error) {
	agent, err := key.ParsePublicKey(agentRef)
	if err != nil {
		return Report{}, err
	}
	tag, err := attestation.ParseAuthTag(tagJSON)
	if err != nil {
		return Report{}, err
	}
	if err := i.verify(agent, tag); err != nil {
		return Report{}, err
	}
	att, err := attestation.New(agent, tag)
	if err != nil {
		return Report{}, err
	}
	r := Report{Agent: agent}
	i.describe(&r, att)
	return r, nil
}

func (i *Inspector) verify(agent key.PublicKey, tag attestation.AuthTag) error {
	att, err := attestation.New(agent, tag)
	if err != nil {
		return err
	}
	if !i.verifier.Verify(att.Owner(), att.Message(), tag.Sig) {
		return shared.New(shared.CodeSignatureInvalid,
			"owner "+att.Owner().Npub()+" did not sign this attestation for agent "+agent.Npub())
	}
	return nil
}

func (i *Inspector) describe(r *Report, att attestation.Attestation) {
	r.Attested = true
	r.Owner = att.Owner()
	r.Conditions = att.Tag().Conditions.String()
	if exp, ok := att.Expiry(); ok {
		r.Expiry = exp
	}
	r.Expired = !att.AdmittedAt(i.clock.Now())
}
