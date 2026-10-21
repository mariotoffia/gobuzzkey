package attestation

import (
	"time"

	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// Attestation binds an auth tag to the agent key it was issued for. The
// signature can only be checked against a specific agent, so the pair is the
// unit of verification.
//
// aggregate-root
type Attestation struct {
	agent key.PublicKey
	tag   AuthTag
}

// New returns a validated attestation.
func New(agent key.PublicKey, tag AuthTag) (Attestation, error) {
	a := Attestation{agent: agent, tag: tag}
	if err := a.Validate(); err != nil {
		return Attestation{}, err
	}
	return a, nil
}

// Validate enforces the invariants that do not need cryptography: both keys
// are set and differ (NIP-OA forbids self-attestation).
func (a Attestation) Validate() error {
	if a.agent.IsZero() || a.tag.Owner.IsZero() {
		return shared.New(shared.CodeInvalidTag, "attestation needs both an agent and an owner key")
	}
	if a.agent == a.tag.Owner {
		return shared.New(shared.CodeSelfAttestation, "owner and agent must be different keys")
	}
	return nil
}

// Agent returns the attested agent key.
func (a Attestation) Agent() key.PublicKey { return a.agent }

// Owner returns the attesting owner key.
func (a Attestation) Owner() key.PublicKey { return a.tag.Owner }

// Tag returns the auth tag.
func (a Attestation) Tag() AuthTag { return a.tag }

// Message returns the BIP-340 message the owner signature must cover.
func (a Attestation) Message() [32]byte { return Message(a.agent, a.tag.Conditions) }

// AdmittedAt reports whether a relay evaluating the attestation at now would
// accept its time conditions.
func (a Attestation) AdmittedAt(now time.Time) bool {
	return a.tag.Conditions.AllowsAdmission(now.Unix())
}

// Expiry returns when the attestation stops covering new events, if it ever does.
func (a Attestation) Expiry() (time.Time, bool) { return a.tag.Conditions.Expiry() }
