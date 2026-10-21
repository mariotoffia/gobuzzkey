// Package identity holds the agent identity: the relay an agent talks to, the
// agent's own secret key and, usually, the owner attestation that admits it.
package identity

import (
	"github.com/mariotoffia/gobuzzkey/domain/attestation"
	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/relay"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// AgentIdentity is everything an agent process needs to act on a Buzz relay:
// BUZZ_RELAY_URL, BUZZ_PRIVATE_KEY and, for an attested agent, BUZZ_AUTH_TAG.
// An agent the relay operator enrolled directly as a member has no
// attestation.
//
// aggregate-root
type AgentIdentity struct {
	relay       relay.URL
	secret      key.SecretKey
	agent       key.PublicKey
	attestation attestation.Attestation
	attested    bool
}

// NewAttested returns an identity admitted through an owner attestation.
// agentPub must be the public key of secret; deriving it needs curve
// arithmetic, so the caller computes it through ports.Signer.
func NewAttested(relayURL relay.URL, secret key.SecretKey, agentPub key.PublicKey, tag attestation.AuthTag) (AgentIdentity, error) {
	att, err := attestation.New(agentPub, tag)
	if err != nil {
		return AgentIdentity{}, shared.Wrap(shared.CodeIdentityInvalid, "identity attestation", err)
	}
	id := AgentIdentity{relay: relayURL, secret: secret, agent: agentPub, attestation: att, attested: true}
	if err := id.Validate(); err != nil {
		return AgentIdentity{}, err
	}
	return id, nil
}

// NewMember returns an identity for an agent that is itself a relay member.
func NewMember(relayURL relay.URL, secret key.SecretKey, agentPub key.PublicKey) (AgentIdentity, error) {
	id := AgentIdentity{relay: relayURL, secret: secret, agent: agentPub}
	if err := id.Validate(); err != nil {
		return AgentIdentity{}, err
	}
	return id, nil
}

// Validate enforces the invariants: a relay, a secret, an agent key and, when
// attested, an attestation issued for that same agent key.
func (id AgentIdentity) Validate() error {
	if id.relay.IsZero() {
		return shared.New(shared.CodeIdentityInvalid, "identity has no relay URL")
	}
	if id.secret.IsZero() || id.agent.IsZero() {
		return shared.New(shared.CodeIdentityInvalid, "identity has no agent key")
	}
	if !id.attested {
		return nil
	}
	if err := id.attestation.Validate(); err != nil {
		return shared.Wrap(shared.CodeIdentityInvalid, "identity attestation", err)
	}
	if id.attestation.Agent() != id.agent {
		return shared.New(shared.CodeIdentityInvalid, "attestation was issued for a different agent key")
	}
	return nil
}

// Relay returns the relay base URL.
func (id AgentIdentity) Relay() relay.URL { return id.relay }

// Secret returns the agent secret key.
func (id AgentIdentity) Secret() key.SecretKey { return id.secret }

// Agent returns the agent public key.
func (id AgentIdentity) Agent() key.PublicKey { return id.agent }

// Attestation returns the owner attestation and whether there is one.
func (id AgentIdentity) Attestation() (attestation.Attestation, bool) {
	return id.attestation, id.attested
}
