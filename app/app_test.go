package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mariotoffia/gobuzzkey/app"
	"github.com/mariotoffia/gobuzzkey/domain/attestation"
	"github.com/mariotoffia/gobuzzkey/domain/clock/clocktest"
	"github.com/mariotoffia/gobuzzkey/domain/identity"
	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/nostr"
	"github.com/mariotoffia/gobuzzkey/domain/relay"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
	"github.com/mariotoffia/gobuzzkey/ports"
)

const now = 1800000000

var ctx = context.Background()

type rig struct {
	crypto *fakeCrypto
	prompt *fakePrompt
	store  *fakeStore
	minter *app.Minter
	owner  key.SecretKey
}

func newRig(t *testing.T) *rig {
	t.Helper()
	owner, err := key.ParseSecretKey("0000000000000000000000000000000000000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{crypto: &fakeCrypto{}, prompt: &fakePrompt{secret: owner}, store: &fakeStore{}, owner: owner}
	r.minter = app.NewMinter(r.crypto, r.crypto, r.crypto, r.prompt, r.store, clocktest.At(now))
	return r
}

func (r *rig) mint(t *testing.T, req app.MintRequest) app.MintResult {
	t.Helper()
	res, err := r.minter.Mint(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestMint_StoresAttestedIdentitySignedByOwner(t *testing.T) {
	r := newRig(t)
	res := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example", Until: time.Unix(now+3600, 0)})

	if len(r.store.saved) != 1 {
		t.Fatalf("saved %d identities", len(r.store.saved))
	}
	id := r.store.saved[0]
	ownerPub, _ := r.crypto.PublicKey(r.owner)
	att, ok := id.Attestation()
	if !ok || att.Owner() != ownerPub || res.Owner != ownerPub {
		t.Fatalf("attestation owner = %v", att.Owner())
	}
	if att.Agent() == ownerPub {
		t.Fatal("agent equals owner")
	}
	if got := att.Tag().Conditions.String(); got != "created_at<1800003600" {
		t.Fatalf("conditions = %q", got)
	}
	if !r.crypto.Verify(ownerPub, att.Message(), att.Tag().Sig) {
		t.Fatal("stored signature does not verify")
	}
	if id.Relay().String() != "https://relay.example" {
		t.Fatalf("relay = %s", id.Relay())
	}
}

func TestMint_WithoutExpiryHasEmptyConditions(t *testing.T) {
	r := newRig(t)
	res := r.mint(t, app.MintRequest{RelayURL: "https://relay.example"})
	att, _ := res.Identity.Attestation()
	if !att.Tag().Conditions.IsEmpty() {
		t.Fatalf("conditions = %q", att.Tag().Conditions)
	}
}

func TestMint_RefusesBeforeReadingTheOwnerSecret(t *testing.T) {
	cases := map[string]struct {
		req  app.MintRequest
		prep func(*rig)
		want shared.Code
	}{
		"bad relay URL":      {app.MintRequest{RelayURL: "relay.example"}, nil, shared.CodeInvalidRelayURL},
		"expiry in the past": {app.MintRequest{RelayURL: "wss://r.example", Until: time.Unix(now-1, 0)}, nil, shared.CodeInvalidConditions},
		"expiry is now":      {app.MintRequest{RelayURL: "wss://r.example", Until: time.Unix(now, 0)}, nil, shared.CodeInvalidConditions},
		"destination exists": {app.MintRequest{RelayURL: "wss://r.example"}, func(r *rig) { r.store.writableErr = shared.New(shared.CodeIdentityExists, "x") }, shared.CodeIdentityExists},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			if tc.prep != nil {
				tc.prep(r)
			}
			_, err := r.minter.Mint(ctx, tc.req)
			if !shared.IsCode(err, tc.want) {
				t.Fatalf("want %s, got %v", tc.want, err)
			}
			if r.prompt.calls != 0 {
				t.Fatal("owner secret was read")
			}
		})
	}
}

func TestMint_PropagatesFailuresAndStoresNothing(t *testing.T) {
	boom := errors.New("boom")
	cases := map[string]struct {
		prep func(*rig)
		want func(error) bool
	}{
		"prompt fails":     {func(r *rig) { r.prompt.err = shared.New(shared.CodeSecretUnavailable, "") }, func(e error) bool { return shared.IsCode(e, shared.CodeSecretUnavailable) }},
		"signer fails":     {func(r *rig) { r.crypto.signErr = boom }, func(e error) bool { return errors.Is(e, boom) }},
		"signature broken": {func(r *rig) { r.crypto.rejectAll = true }, func(e error) bool { return shared.IsCode(e, shared.CodeSignatureInvalid) }},
		"store fails":      {func(r *rig) { r.store.saveErr = boom }, func(e error) bool { return errors.Is(e, boom) }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			tc.prep(r)
			_, err := r.minter.Mint(ctx, app.MintRequest{RelayURL: "wss://r.example"})
			if !tc.want(err) {
				t.Fatalf("unexpected error %v", err)
			}
			if len(r.store.saved) != 0 {
				t.Fatal("identity stored after a failure")
			}
		})
	}
}

// record renders a minted identity as a stored record.
func record(id identity.AgentIdentity) ports.IdentityRecord {
	rec := ports.IdentityRecord{RelayURL: id.Relay().String(), PrivateKey: id.Secret().Nsec()}
	if att, ok := id.Attestation(); ok {
		rec.AuthTag = att.Tag().String()
	}
	return rec
}

func TestOpen_VerifiesTheStoredAttestation(t *testing.T) {
	r := newRig(t)
	minted := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example", Until: time.Unix(now+60, 0)})
	insp := app.NewInspector(r.crypto, r.crypto, clocktest.At(now))

	id, err := insp.Open(ctx, fakeSource{rec: record(minted.Identity)})
	if err != nil {
		t.Fatal(err)
	}
	rep := insp.Inspect(id)
	if !rep.Attested || rep.Expired || rep.Owner != minted.Owner || rep.Expiry.Unix() != now+60 {
		t.Fatalf("report = %+v", rep)
	}

	later := app.NewInspector(r.crypto, r.crypto, clocktest.At(now+60))
	id, _ = later.Open(ctx, fakeSource{rec: record(minted.Identity)})
	if !later.Inspect(id).Expired {
		t.Fatal("expired attestation reported admissible")
	}
}

func TestOpen_RejectsMismatchedOrBrokenRecords(t *testing.T) {
	r := newRig(t)
	a := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example"}).Identity
	b := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example"}).Identity
	insp := app.NewInspector(r.crypto, r.crypto, clocktest.At(now))

	swapped := record(a)
	swapped.AuthTag = record(b).AuthTag // tag issued for another agent
	badRelay := record(a)
	badRelay.RelayURL = "relay.example"
	badKey := record(a)
	badKey.PrivateKey = "nsec1nope"
	badTag := record(a)
	badTag.AuthTag = `["auth"]`

	cases := map[string]struct {
		rec  ports.IdentityRecord
		want shared.Code
	}{
		"tag for another agent": {swapped, shared.CodeSignatureInvalid},
		"bad relay":             {badRelay, shared.CodeIdentityInvalid},
		"bad private key":       {badKey, shared.CodeIdentityInvalid},
		"malformed tag":         {badTag, shared.CodeIdentityInvalid},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := insp.Open(ctx, fakeSource{rec: tc.rec}); !shared.IsCode(err, tc.want) {
				t.Fatalf("want %s, got %v", tc.want, err)
			}
		})
	}
}

func TestOpen_RecordWithoutTagIsAMember(t *testing.T) {
	r := newRig(t)
	rec := record(r.mint(t, app.MintRequest{RelayURL: "wss://relay.example"}).Identity)
	rec.AuthTag = ""
	insp := app.NewInspector(r.crypto, r.crypto, clocktest.At(now))
	id, err := insp.Open(ctx, fakeSource{rec: rec})
	if err != nil {
		t.Fatal(err)
	}
	if insp.Inspect(id).Attested {
		t.Fatal("member reported as attested")
	}
}

func TestVerifyTag_ChecksTheNamedAgent(t *testing.T) {
	r := newRig(t)
	id := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example"}).Identity
	att, _ := id.Attestation()
	insp := app.NewInspector(r.crypto, r.crypto, clocktest.At(now))

	rep, err := insp.VerifyTag(att.Tag().String(), id.Agent().Npub())
	if err != nil || !rep.Attested || rep.Agent != id.Agent() {
		t.Fatalf("report = %+v, %v", rep, err)
	}
	if _, err := insp.VerifyTag(att.Tag().String(), att.Owner().Hex()); !shared.IsCode(err, shared.CodeSelfAttestation) {
		t.Fatalf("owner as agent: %v", err)
	}
	other := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example"}).Identity
	if _, err := insp.VerifyTag(att.Tag().String(), other.Agent().Npub()); !shared.IsCode(err, shared.CodeSignatureInvalid) {
		t.Fatalf("other agent: %v", err)
	}
}

func TestCheck_ListsReadsAndPostsAsTheAgent(t *testing.T) {
	r := newRig(t)
	id := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example"}).Identity
	fr := &fakeRelay{
		channels: []relay.Channel{{ID: "c-2", Name: "zeta"}, {ID: "c-1", Name: "teos-grok"}},
		history:  map[string]int{"c-1": 2},
	}
	rep, err := app.NewChecker(r.crypto, clocktest.At(now)).Check(ctx, id, fr, app.CheckRequest{Channel: "TEOS-GROK", Send: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Admitted || len(rep.Channels) != 2 || rep.Channels[0].Name != "teos-grok" {
		t.Fatalf("channels = %+v", rep.Channels)
	}
	if rep.Channel.ID != "c-1" || rep.Recent != 2 || !rep.Posted.Accepted {
		t.Fatalf("report = %+v", rep)
	}
	if fr.queries[1].Limit != 3 {
		t.Fatalf("history limit = %d, want the default 3", fr.queries[1].Limit)
	}
	ev := fr.published[0]
	if ev.Kind != nostr.KindChannelMessage || ev.PubKey != id.Agent().Hex() || ev.Content != "hello" || ev.CreatedAt != now {
		t.Fatalf("event = %+v", ev)
	}
	if h, _ := ev.TagValue("h"); h != "c-1" {
		t.Fatalf("h tag = %q", h)
	}
	sig, _ := key.ParseSignatureHex(ev.Sig)
	if !r.crypto.Verify(id.Agent(), ev.ComputeID(), sig) {
		t.Fatal("posted event is not signed by the agent")
	}
	att, _ := id.Attestation()
	if got, _ := ev.TagValue("auth"); got != att.Owner().Hex() {
		t.Fatalf("posted event carries auth tag owner %q, want %s", got, att.Owner().Hex())
	}
}

func TestCheck_AmbiguousNameIsRefusedAndIDResolves(t *testing.T) {
	r := newRig(t)
	id := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example"}).Identity
	fr := &fakeRelay{channels: []relay.Channel{{ID: "c-2", Name: "general"}, {ID: "c-1", Name: "general"}}}
	checker := app.NewChecker(r.crypto, clocktest.At(now))

	_, err := checker.Check(ctx, id, fr, app.CheckRequest{Channel: "general", Send: "hi"})
	if !shared.IsCode(err, shared.CodeChannelAmbiguous) {
		t.Fatalf("want CHANNEL_AMBIGUOUS, got %v", err)
	}
	if len(fr.published) != 0 {
		t.Fatal("posted to a guessed channel")
	}
	rep, err := checker.Check(ctx, id, fr, app.CheckRequest{Channel: "c-2"})
	if err != nil || rep.Channel.ID != "c-2" {
		t.Fatalf("by ID: %+v, %v", rep.Channel, err)
	}
	if rep.Channels[0].ID != "c-1" {
		t.Fatalf("channels with equal names are not ordered by ID: %+v", rep.Channels)
	}
}

func TestCheck_RejectsInvalidUTF8BeforeSigning(t *testing.T) {
	r := newRig(t)
	id := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example"}).Identity
	fr := &fakeRelay{channels: []relay.Channel{{ID: "c-1", Name: "x"}}}
	_, err := app.NewChecker(r.crypto, clocktest.At(now)).Check(ctx, id, fr, app.CheckRequest{Channel: "x", Send: "bad \xff"})
	if !shared.IsCode(err, shared.CodeInvalidEvent) || len(fr.published) != 0 {
		t.Fatalf("want INVALID_EVENT and no post, got %v", err)
	}
}

func TestCheck_AuthTagOmittedWhenConditionsExcludeTheKind(t *testing.T) {
	r := newRig(t)
	// A kind-1-only attestation does not cover kind 9, so the tag is left out.
	agentSK, _ := r.crypto.Generate()
	agent, _ := r.crypto.PublicKey(agentSK)
	ownerPub, _ := r.crypto.PublicKey(r.owner)
	conds, _ := attestation.ParseConditions("kind=1")
	sig, _ := r.crypto.Sign(r.owner, attestation.Message(agent, conds))
	u, _ := relay.ParseURL("wss://relay.example")
	id, err := identity.NewAttested(u, agentSK, agent, attestation.AuthTag{Owner: ownerPub, Conditions: conds, Sig: sig})
	if err != nil {
		t.Fatal(err)
	}
	fr := &fakeRelay{channels: []relay.Channel{{ID: "c-1", Name: "x"}}}
	if _, err := app.NewChecker(r.crypto, clocktest.At(now)).Check(ctx, id, fr, app.CheckRequest{Channel: "x", Send: "hi"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := fr.published[0].TagValue("auth"); ok {
		t.Fatal("auth tag attached to an event its conditions do not cover")
	}
}

func TestCheck_Failures(t *testing.T) {
	r := newRig(t)
	id := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example"}).Identity
	checker := app.NewChecker(r.crypto, clocktest.At(now))

	t.Run("channel not visible", func(t *testing.T) {
		rep, err := checker.Check(ctx, id, &fakeRelay{}, app.CheckRequest{Channel: "teos-grok"})
		if !shared.IsCode(err, shared.CodeChannelNotVisible) || !rep.Admitted {
			t.Fatalf("want CHANNEL_NOT_VISIBLE after admission, got %v %+v", err, rep)
		}
	})
	t.Run("relay refuses", func(t *testing.T) {
		fr := &fakeRelay{queryErr: shared.New(shared.CodeRelayMembershipRequired, "no")}
		rep, err := checker.Check(ctx, id, fr, app.CheckRequest{})
		if !shared.IsCode(err, shared.CodeRelayMembershipRequired) || rep.Admitted {
			t.Fatalf("got %v %+v", err, rep)
		}
	})
	t.Run("write rejected", func(t *testing.T) {
		fr := &fakeRelay{channels: []relay.Channel{{ID: "c-1", Name: "x"}}, reject: "restricted"}
		_, err := checker.Check(ctx, id, fr, app.CheckRequest{Channel: "x", Send: "hi"})
		if !shared.IsCode(err, shared.CodeRelayRejected) {
			t.Fatalf("want RELAY_REJECTED, got %v", err)
		}
	})
}

func TestCheck_ExpiredAttestationNeverReachesTheRelay(t *testing.T) {
	r := newRig(t)
	id := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example", Until: time.Unix(now+1, 0)}).Identity
	fr := &fakeRelay{}
	_, err := app.NewChecker(r.crypto, clocktest.At(now+1)).Check(ctx, id, fr, app.CheckRequest{})
	if !shared.IsCode(err, shared.CodeAttestationExpired) {
		t.Fatalf("want ATTESTATION_EXPIRED, got %v", err)
	}
	if len(fr.queries) != 0 {
		t.Fatal("relay was queried")
	}
}

func TestMintedConditions_AreValidNIPOA(t *testing.T) {
	r := newRig(t)
	att, _ := r.mint(t, app.MintRequest{RelayURL: "wss://r.example", Until: time.Unix(now+10, 0)}).Identity.Attestation()
	if _, err := attestation.ParseConditions(att.Tag().Conditions.String()); err != nil {
		t.Fatal(err)
	}
}
