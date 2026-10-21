package ports

import (
	"context"

	"github.com/mariotoffia/gobuzzkey/domain/identity"
	"github.com/mariotoffia/gobuzzkey/domain/key"
)

// IdentityStore persists an agent identity. Implementations never overwrite
// an existing identity (shared.CodeIdentityExists).
type IdentityStore interface {
	// Writable reports, before any secret is read, whether Save can succeed.
	Writable(ctx context.Context) error
	Save(ctx context.Context, id identity.AgentIdentity) error
}

// IdentityRecord is a stored identity before the application has checked
// that the secret key and the attested agent key belong together.
type IdentityRecord struct {
	RelayURL   string
	PrivateKey string
	AuthTag    string // empty when the agent is a direct relay member
}

// IdentitySource reads a stored identity.
type IdentitySource interface {
	Load(ctx context.Context) (IdentityRecord, error)
}

// SecretPrompt obtains the owner's secret key. It is used once, to sign the
// attestation, and is never stored.
type SecretPrompt interface {
	OwnerSecret(ctx context.Context) (key.SecretKey, error)
}
