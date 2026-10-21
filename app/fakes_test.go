package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/binary"

	"github.com/mariotoffia/gobuzzkey/domain/identity"
	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/nostr"
	"github.com/mariotoffia/gobuzzkey/domain/relay"
	"github.com/mariotoffia/gobuzzkey/ports"
)

// fakeCrypto is a deterministic stand-in for BIP-340. A "signature" is a hash
// of the public key and message, so it verifies only for that key and
// message, which is all the use cases rely on.
type fakeCrypto struct {
	next      uint64
	signErr   error
	rejectAll bool
}

func (f *fakeCrypto) PublicKey(sk key.SecretKey) (key.PublicKey, error) {
	b := sk.Bytes()
	for i := uint32(0); ; i++ {
		h := sha256.Sum256(append(b[:], byte(i), byte(i>>8)))
		if pub, err := key.PublicKeyFromBytes(h); err == nil {
			return pub, nil
		}
	}
}

func (f *fakeCrypto) Sign(sk key.SecretKey, msg [32]byte) (key.Signature, error) {
	if f.signErr != nil {
		return key.Signature{}, f.signErr
	}
	pub, _ := f.PublicKey(sk)
	return fakeSig(pub, msg), nil
}

func (f *fakeCrypto) Verify(pub key.PublicKey, msg [32]byte, sig key.Signature) bool {
	return !f.rejectAll && fakeSig(pub, msg) == sig
}

func (f *fakeCrypto) Generate() (key.SecretKey, error) {
	f.next++
	var b [key.Size]byte
	b[0] = 0x42
	binary.BigEndian.PutUint64(b[24:], f.next)
	return key.SecretKeyFromBytes(b)
}

func fakeSig(pub key.PublicKey, msg [32]byte) key.Signature {
	p := pub.Bytes()
	a := sha256.Sum256(append(p[:], msg[:]...))
	b := sha256.Sum256(append(msg[:], p[:]...))
	var s key.Signature
	copy(s[:32], a[:])
	copy(s[32:], b[:])
	return s
}

type fakePrompt struct {
	secret key.SecretKey
	err    error
	calls  int
}

func (f *fakePrompt) OwnerSecret(context.Context) (key.SecretKey, error) {
	f.calls++
	return f.secret, f.err
}

type fakeStore struct {
	writableErr error
	saveErr     error
	saved       []identity.AgentIdentity
}

func (f *fakeStore) Writable(context.Context) error { return f.writableErr }

func (f *fakeStore) Save(_ context.Context, id identity.AgentIdentity) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, id)
	return nil
}

type fakeSource struct {
	rec ports.IdentityRecord
	err error
}

func (f fakeSource) Load(context.Context) (ports.IdentityRecord, error) { return f.rec, f.err }

type fakeRelay struct {
	channels  []relay.Channel
	history   map[string]int
	queryErr  error
	reject    string // non-empty: Publish answers accepted=false with this message
	queries   []nostr.Filter
	published []nostr.Event
}

func (f *fakeRelay) Query(_ context.Context, filter nostr.Filter) ([]nostr.Event, error) {
	f.queries = append(f.queries, filter)
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	var out []nostr.Event
	if filter.Kinds[0] == nostr.KindChannelMetadata {
		for _, ch := range f.channels {
			out = append(out, nostr.Event{Kind: nostr.KindChannelMetadata, Tags: [][]string{{"d", ch.ID}, {"name", ch.Name}}})
		}
		return out, nil
	}
	for range f.history[filter.Tags["h"][0]] {
		out = append(out, nostr.Event{Kind: nostr.KindChannelMessage})
	}
	return out, nil
}

func (f *fakeRelay) Publish(_ context.Context, ev nostr.Event) (relay.WriteResult, error) {
	f.published = append(f.published, ev)
	if f.reject != "" {
		return relay.WriteResult{EventID: ev.ID, Message: f.reject}, nil
	}
	return relay.WriteResult{EventID: ev.ID, Accepted: true}, nil
}

var (
	_ ports.Signer         = (*fakeCrypto)(nil)
	_ ports.Verifier       = (*fakeCrypto)(nil)
	_ ports.KeyGenerator   = (*fakeCrypto)(nil)
	_ ports.SecretPrompt   = (*fakePrompt)(nil)
	_ ports.IdentityStore  = (*fakeStore)(nil)
	_ ports.IdentitySource = fakeSource{}
	_ ports.Relay          = (*fakeRelay)(nil)
)
