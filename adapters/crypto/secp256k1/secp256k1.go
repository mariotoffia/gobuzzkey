// Package secp256k1 implements the crypto ports with BIP-340 Schnorr
// signatures over secp256k1 (github.com/btcsuite/btcd/btcec/v2).
package secp256k1

import (
	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/ports"
)

// Crypto is the stateless BIP-340 implementation of ports.Signer,
// ports.Verifier and ports.KeyGenerator.
type Crypto struct{}

// New returns the crypto adapter.
func New() Crypto { return Crypto{} }

// PublicKey derives the x-only public key of sk.
func (Crypto) PublicKey(sk key.SecretKey) (key.PublicKey, error) { return derivePublic(sk) }

// Sign signs msg with sk.
func (Crypto) Sign(sk key.SecretKey, msg [32]byte) (key.Signature, error) { return sign(sk, msg) }

// Verify checks sig over msg for pub.
func (Crypto) Verify(pub key.PublicKey, msg [32]byte, sig key.Signature) bool {
	return verify(pub, msg, sig)
}

// Generate returns a fresh secret key from crypto/rand.
func (Crypto) Generate() (key.SecretKey, error) { return generate() }

var (
	_ ports.Signer       = Crypto{}
	_ ports.Verifier     = Crypto{}
	_ ports.KeyGenerator = Crypto{}
)
