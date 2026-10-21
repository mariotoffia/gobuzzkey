// Package ports declares the contracts between the application core and the
// outside world. Adapters implement them; the composition root wires them.
package ports

import "github.com/mariotoffia/gobuzzkey/domain/key"

// Signer produces BIP-340 Schnorr signatures and derives x-only public keys.
type Signer interface {
	// PublicKey derives the x-only public key of sk.
	PublicKey(sk key.SecretKey) (key.PublicKey, error)
	// Sign signs a 32-byte message with sk.
	Sign(sk key.SecretKey, msg [32]byte) (key.Signature, error)
}

// Verifier checks BIP-340 Schnorr signatures.
type Verifier interface {
	Verify(pub key.PublicKey, msg [32]byte, sig key.Signature) bool
}

// KeyGenerator creates fresh secret keys from a cryptographic source.
type KeyGenerator interface {
	Generate() (key.SecretKey, error)
}
