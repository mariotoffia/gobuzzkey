package secp256k1_test

import "github.com/btcsuite/btcd/btcec/v2/schnorr"

func btcecAccepts(x [32]byte) bool {
	_, err := schnorr.ParsePubKey(x[:])
	return err == nil
}
