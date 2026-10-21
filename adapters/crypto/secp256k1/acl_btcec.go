package secp256k1

// Anti-corruption layer: the only file that imports btcec. Exported
// signatures in this package use domain types only.

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"

	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

func derivePublic(sk key.SecretKey) (key.PublicKey, error) {
	b := sk.Bytes()
	priv, _ := btcec.PrivKeyFromBytes(b[:])
	var x [key.Size]byte
	copy(x[:], schnorr.SerializePubKey(priv.PubKey()))
	return key.PublicKeyFromBytes(x)
}

func sign(sk key.SecretKey, msg [32]byte) (key.Signature, error) {
	b := sk.Bytes()
	priv, _ := btcec.PrivKeyFromBytes(b[:])
	sig, err := schnorr.Sign(priv, msg[:])
	if err != nil {
		return key.Signature{}, shared.Wrap(shared.CodeInvalidKey, "schnorr sign", err)
	}
	var out key.Signature
	copy(out[:], sig.Serialize())
	return out, nil
}

func verify(pub key.PublicKey, msg [32]byte, sig key.Signature) bool {
	pb := pub.Bytes()
	pk, err := schnorr.ParsePubKey(pb[:])
	if err != nil {
		return false
	}
	s, err := schnorr.ParseSignature(sig[:])
	if err != nil {
		return false
	}
	return s.Verify(msg[:], pk)
}

func generate() (key.SecretKey, error) {
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		return key.SecretKey{}, shared.Wrap(shared.CodeInvalidKey, "generate secret key", err)
	}
	var b [key.Size]byte
	priv.Key.PutBytes(&b)
	return key.SecretKeyFromBytes(b)
}
