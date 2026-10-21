package secp256k1_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mariotoffia/gobuzzkey/adapters/crypto/secp256k1"
	"github.com/mariotoffia/gobuzzkey/domain/attestation"
	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/nostr"
)

// NIP-OA test vectors.
const (
	ownerSK  = "0000000000000000000000000000000000000000000000000000000000000001"
	ownerPub = "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"
	agentSK  = "0000000000000000000000000000000000000000000000000000000000000002"
	agentPub = "c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5"
	specTag  = `["auth","79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","kind=1&created_at<1713957000","8b7df2575caf0a108374f8471722b233c53f9ff827a8b0f91861966c3b9dd5cb2e189eae9f49d72187674c2f5bd244145e10ff86c9f257ffe65a1ee5f108b369"]`
	specEvt  = `{"id":"d892a65e7677e0554ebb70ee16deeb6a0727dba46450fb4bc001291d7bff971b","pubkey":"c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5","created_at":1713956400,"kind":1,"tags":[["auth","79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","kind=1&created_at<1713957000","8b7df2575caf0a108374f8471722b233c53f9ff827a8b0f91861966c3b9dd5cb2e189eae9f49d72187674c2f5bd244145e10ff86c9f257ffe65a1ee5f108b369"]],"content":"owner-attested agent event","sig":"7fd38992b70b5e9e113644e51b4c8ee2227f3bdd402b1855f8786c0600394ab3ec2621742a7bad0b0000b93d4d1ae6e39525f286a3c1029f43f46c3359a6c76f"}`
)

func secret(t *testing.T, h string) key.SecretKey {
	t.Helper()
	sk, err := key.ParseSecretKey(h)
	if err != nil {
		t.Fatal(err)
	}
	return sk
}

func TestPublicKey_MatchesSpecKeys(t *testing.T) {
	c := secp256k1.New()
	for sk, want := range map[string]string{ownerSK: ownerPub, agentSK: agentPub} {
		pub, err := c.PublicKey(secret(t, sk))
		if err != nil {
			t.Fatal(err)
		}
		if pub.Hex() != want {
			t.Fatalf("pub(%s) = %s, want %s", sk[60:], pub.Hex(), want)
		}
	}
}

func TestVerify_AcceptsSpecAuthTagSignature(t *testing.T) {
	c := secp256k1.New()
	tag, err := attestation.ParseAuthTag(specTag)
	if err != nil {
		t.Fatal(err)
	}
	agent, _ := key.ParsePublicKeyHex(agentPub)
	if !c.Verify(tag.Owner, attestation.Message(agent, tag.Conditions), tag.Sig) {
		t.Fatal("spec auth signature rejected")
	}
	other, _ := key.ParsePublicKeyHex(ownerPub)
	if c.Verify(tag.Owner, attestation.Message(other, tag.Conditions), tag.Sig) {
		t.Fatal("signature accepted for another agent")
	}
}

// BIP-340 nonces depend on auxiliary randomness, so a fresh signature differs
// from the NIP-OA vector bytes; it must still verify for the vector message.
func TestSign_SpecMessageProducesVerifiableSignature(t *testing.T) {
	c := secp256k1.New()
	tag, _ := attestation.ParseAuthTag(specTag)
	agent, _ := key.ParsePublicKeyHex(agentPub)
	sig, err := c.Sign(secret(t, ownerSK), attestation.Message(agent, tag.Conditions))
	if err != nil {
		t.Fatal(err)
	}
	if !c.Verify(tag.Owner, attestation.Message(agent, tag.Conditions), sig) {
		t.Fatal("own signature does not verify")
	}
}

func TestVerify_AcceptsSpecEventSignature(t *testing.T) {
	var ev nostr.Event
	if err := json.Unmarshal([]byte(specEvt), &ev); err != nil {
		t.Fatal(err)
	}
	pub, _ := key.ParsePublicKeyHex(ev.PubKey)
	sig, _ := key.ParseSignatureHex(ev.Sig)
	if !secp256k1.New().Verify(pub, ev.ComputeID(), sig) {
		t.Fatal("spec event signature rejected")
	}
}

func TestGenerate_ProducesDistinctUsableKeys(t *testing.T) {
	c := secp256k1.New()
	seen := map[[key.Size]byte]bool{}
	for range 16 {
		sk, err := c.Generate()
		if err != nil {
			t.Fatal(err)
		}
		if seen[sk.Bytes()] {
			t.Fatal("duplicate key")
		}
		seen[sk.Bytes()] = true
		pub, err := c.PublicKey(sk)
		if err != nil {
			t.Fatal(err)
		}
		msg := sha256.Sum256([]byte("m"))
		sig, err := c.Sign(sk, msg)
		if err != nil || !c.Verify(pub, msg, sig) {
			t.Fatalf("generated key cannot sign: %v", err)
		}
	}
}

// The domain validates x-only keys with stdlib arithmetic; btcec is the
// reference. They must agree on every input.
func TestDomainCurveCheck_AgreesWithBtcec(t *testing.T) {
	c := secp256k1.New()
	sig := strings.Repeat("00", key.SignatureSize)
	zeroSig, _ := key.ParseSignatureHex(sig)
	for i := range 256 {
		h := sha256.Sum256([]byte{byte(i), byte(i >> 8)})
		_, domainErr := key.PublicKeyFromBytes(h)
		btcecOK := btcecAccepts(h)
		if (domainErr == nil) != btcecOK {
			t.Fatalf("x=%s: domain ok=%v, btcec ok=%v", hex.EncodeToString(h[:]), domainErr == nil, btcecOK)
		}
	}
	_ = c.Verify(key.PublicKey{}, [32]byte{}, zeroSig) // zero key must not panic
}
