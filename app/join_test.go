package app_test

import (
	"testing"
	"time"

	"github.com/mariotoffia/gobuzzkey/app"
	"github.com/mariotoffia/gobuzzkey/domain/clock/clocktest"
	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/nostr"
	"github.com/mariotoffia/gobuzzkey/domain/relay"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

func TestJoin_OpenChannelSendsAJoinRequestAsTheAgent(t *testing.T) {
	r := newRig(t)
	id := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example"}).Identity
	fr := &fakeRelay{channels: []relay.Channel{{ID: "c-1", Name: "teos-grok"}}}

	rep, err := app.NewChecker(r.crypto, clocktest.At(now)).Join(ctx, id, fr, "teos-grok")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Changed || rep.Channel.ID != "c-1" || len(fr.published) != 1 {
		t.Fatalf("report = %+v, published %d", rep, len(fr.published))
	}
	ev := fr.published[0]
	if ev.Kind != nostr.KindJoinRequest || ev.PubKey != id.Agent().Hex() || ev.Content != "" {
		t.Fatalf("event = %+v", ev)
	}
	if h, _ := ev.TagValue("h"); h != "c-1" {
		t.Fatalf("h tag = %q", h)
	}
	sig, _ := key.ParseSignatureHex(ev.Sig)
	if !r.crypto.Verify(id.Agent(), ev.ComputeID(), sig) {
		t.Fatal("join request is not signed by the agent")
	}
}

func TestJoin_MemberAlreadyPublishesNothing(t *testing.T) {
	r := newRig(t)
	id := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example"}).Identity
	fr := &fakeRelay{
		channels: []relay.Channel{{ID: "c-1", Name: "teos-grok", Private: true}},
		members:  map[string][]string{"c-1": {id.Agent().Hex()}},
	}
	rep, err := app.NewChecker(r.crypto, clocktest.At(now)).Join(ctx, id, fr, "teos-grok")
	if err != nil || rep.Changed || len(fr.published) != 0 {
		t.Fatalf("report = %+v, err %v, published %d", rep, err, len(fr.published))
	}
}

func TestJoin_PrivateChannelIsRefusedWithoutPublishing(t *testing.T) {
	r := newRig(t)
	id := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example"}).Identity
	fr := &fakeRelay{channels: []relay.Channel{{ID: "c-1", Name: "teos-grok", Private: true}}}
	_, err := app.NewChecker(r.crypto, clocktest.At(now)).Join(ctx, id, fr, "teos-grok")
	if !shared.IsCode(err, shared.CodeChannelNotVisible) || len(fr.published) != 0 {
		t.Fatalf("want CHANNEL_NOT_VISIBLE and no join request, got %v, published %d", err, len(fr.published))
	}
}

func TestJoin_Failures(t *testing.T) {
	r := newRig(t)
	id := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example"}).Identity
	checker := app.NewChecker(r.crypto, clocktest.At(now))
	cases := map[string]struct {
		relay *fakeRelay
		want  shared.Code
	}{
		"channel not visible": {&fakeRelay{}, shared.CodeChannelNotVisible},
		"relay refuses":       {&fakeRelay{queryErr: shared.New(shared.CodeRelayMembershipRequired, "no")}, shared.CodeRelayMembershipRequired},
		"join rejected":       {&fakeRelay{channels: []relay.Channel{{ID: "c-1", Name: "x"}}, reject: "restricted"}, shared.CodeRelayRejected},
	}
	for name, tc := range cases {
		if _, err := checker.Join(ctx, id, tc.relay, "x"); !shared.IsCode(err, tc.want) {
			t.Errorf("%s: want %s, got %v", name, tc.want, err)
		}
	}
}

func TestJoin_ExpiredAttestationNeverReachesTheRelay(t *testing.T) {
	r := newRig(t)
	id := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example", Until: time.Unix(now+1, 0)}).Identity
	fr := &fakeRelay{channels: []relay.Channel{{ID: "c-1", Name: "x"}}}
	_, err := app.NewChecker(r.crypto, clocktest.At(now+1)).Join(ctx, id, fr, "x")
	if !shared.IsCode(err, shared.CodeAttestationExpired) || len(fr.queries) != 0 {
		t.Fatalf("want ATTESTATION_EXPIRED before any query, got %v after %d queries", err, len(fr.queries))
	}
}

func TestLeave_MemberSendsALeaveRequestAndNonMemberNothing(t *testing.T) {
	r := newRig(t)
	id := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example"}).Identity
	checker := app.NewChecker(r.crypto, clocktest.At(now))
	fr := &fakeRelay{
		channels: []relay.Channel{{ID: "c-1", Name: "teos-grok", Private: true}},
		members:  map[string][]string{"c-1": {id.Agent().Hex()}},
	}
	rep, err := checker.Leave(ctx, id, fr, "teos-grok")
	if err != nil || !rep.Changed || len(fr.published) != 1 || fr.published[0].Kind != nostr.KindLeaveRequest {
		t.Fatalf("member: report = %+v, err %v, published %+v", rep, err, fr.published)
	}
	if h, _ := fr.published[0].TagValue("h"); h != "c-1" {
		t.Fatalf("h tag = %q", h)
	}

	fr = &fakeRelay{channels: []relay.Channel{{ID: "c-1", Name: "teos-grok"}}}
	rep, err = checker.Leave(ctx, id, fr, "teos-grok")
	if err != nil || rep.Changed || len(fr.published) != 0 {
		t.Fatalf("non-member: report = %+v, err %v, published %d", rep, err, len(fr.published))
	}
}

func TestReusable_SameRelayInAnySpellingIsKept(t *testing.T) {
	r := newRig(t)
	id := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example", Until: time.Unix(now+3600, 0)}).Identity
	ins := app.NewInspector(r.crypto, r.crypto, clocktest.At(now))
	for _, u := range []string{"wss://relay.example", "https://relay.example/", "WSS://Relay.Example"} {
		if err := ins.Reusable(id, u); err != nil {
			t.Errorf("%q: %v", u, err)
		}
	}
}

func TestReusable_RefusesAnotherRelayOrAnExpiredAttestation(t *testing.T) {
	r := newRig(t)
	id := r.mint(t, app.MintRequest{RelayURL: "wss://relay.example", Until: time.Unix(now+3600, 0)}).Identity
	cases := map[string]struct {
		at    int64
		relay string
		want  shared.Code
	}{
		"other relay":      {now, "wss://other.example", shared.CodeIdentityExists},
		"other port":       {now, "wss://relay.example:8443", shared.CodeIdentityExists},
		"expired":          {now + 3600, "wss://relay.example", shared.CodeAttestationExpired},
		"invalid relay":    {now, "ftp://relay.example", shared.CodeInvalidRelayURL},
		"one second early": {now + 3599, "wss://relay.example", ""},
	}
	for name, tc := range cases {
		err := app.NewInspector(r.crypto, r.crypto, clocktest.At(tc.at)).Reusable(id, tc.relay)
		if tc.want == "" {
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
			continue
		}
		if !shared.IsCode(err, tc.want) {
			t.Errorf("%s: want %s, got %v", name, tc.want, err)
		}
	}
}
