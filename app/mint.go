// Package app holds the use cases: mint an attested agent identity, open and
// inspect a stored one, and check it against a live relay. It depends on
// domain and ports only; adapters are wired in cmd/gobuzzkey.
package app

import (
	"context"
	"time"

	"github.com/mariotoffia/gobuzzkey/domain/attestation"
	"github.com/mariotoffia/gobuzzkey/domain/clock"
	"github.com/mariotoffia/gobuzzkey/domain/identity"
	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/relay"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
	"github.com/mariotoffia/gobuzzkey/ports"
)

// Minter creates a fresh agent key and an owner attestation for it.
type Minter struct {
	signer   ports.Signer
	verifier ports.Verifier
	keys     ports.KeyGenerator
	owner    ports.SecretPrompt
	store    ports.IdentityStore
	clock    clock.Clock
}

// NewMinter wires the mint use case.
func NewMinter(
	signer ports.Signer,
	verifier ports.Verifier,
	keys ports.KeyGenerator,
	owner ports.SecretPrompt,
	store ports.IdentityStore,
	clk clock.Clock,
) *Minter {
	return &Minter{signer: signer, verifier: verifier, keys: keys, owner: owner, store: store, clock: clk}
}

// MintRequest describes the identity to create.
type MintRequest struct {
	// RelayURL is the relay the agent will use (ws, wss, http or https).
	RelayURL string
	// Until, when set, makes the attestation cover only events created
	// before that instant (a created_at< clause). It must be in the future.
	Until time.Time
}

// MintResult is the stored identity plus the owner key that attested it.
type MintResult struct {
	Identity identity.AgentIdentity
	Owner    key.PublicKey
}

// Mint reads the owner secret, generates the agent key, signs the
// attestation, checks the signature and stores the identity. The owner
// secret is used for one signature and is not retained.
func (m *Minter) Mint(ctx context.Context, req MintRequest) (MintResult, error) {
	relayURL, err := relay.ParseURL(req.RelayURL)
	if err != nil {
		return MintResult{}, err
	}
	conds, err := m.conditions(req.Until)
	if err != nil {
		return MintResult{}, err
	}
	if err := m.store.Writable(ctx); err != nil {
		return MintResult{}, err
	}

	ownerSecret, err := m.owner.OwnerSecret(ctx)
	if err != nil {
		return MintResult{}, err
	}
	ownerPub, err := m.signer.PublicKey(ownerSecret)
	if err != nil {
		return MintResult{}, err
	}

	agentSecret, err := m.keys.Generate()
	if err != nil {
		return MintResult{}, err
	}
	agentPub, err := m.signer.PublicKey(agentSecret)
	if err != nil {
		return MintResult{}, err
	}

	sig, err := m.signer.Sign(ownerSecret, attestation.Message(agentPub, conds))
	if err != nil {
		return MintResult{}, err
	}
	tag := attestation.AuthTag{Owner: ownerPub, Conditions: conds, Sig: sig}
	if !m.verifier.Verify(ownerPub, attestation.Message(agentPub, conds), sig) {
		return MintResult{}, shared.New(shared.CodeSignatureInvalid, "freshly signed attestation does not verify")
	}

	id, err := identity.NewAttested(relayURL, agentSecret, agentPub, tag)
	if err != nil {
		return MintResult{}, err
	}
	if err := m.store.Save(ctx, id); err != nil {
		return MintResult{}, err
	}
	return MintResult{Identity: id, Owner: ownerPub}, nil
}

func (m *Minter) conditions(until time.Time) (attestation.Conditions, error) {
	if until.IsZero() {
		return attestation.Conditions{}, nil
	}
	if !until.After(m.clock.Now()) {
		return attestation.Conditions{}, shared.New(shared.CodeInvalidConditions,
			"expiry "+until.UTC().Format(time.RFC3339)+" is not in the future")
	}
	return attestation.Build(attestation.Until(until))
}
