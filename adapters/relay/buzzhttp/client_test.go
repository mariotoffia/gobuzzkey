package buzzhttp_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/mariotoffia/gobuzzkey/adapters/crypto/secp256k1"
	"github.com/mariotoffia/gobuzzkey/adapters/relay/buzzhttp"
	"github.com/mariotoffia/gobuzzkey/domain/attestation"
	"github.com/mariotoffia/gobuzzkey/domain/clock/clocktest"
	"github.com/mariotoffia/gobuzzkey/domain/identity"
	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/nostr"
	"github.com/mariotoffia/gobuzzkey/domain/relay"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
	"github.com/mariotoffia/gobuzzkey/testutil/fakerelay"
)

const now = 1800000000

type keys struct {
	owner, agent       key.SecretKey
	ownerPub, agentPub key.PublicKey
}

func newKeys(t *testing.T) keys {
	t.Helper()
	c := secp256k1.New()
	var k keys
	var err error
	if k.owner, err = c.Generate(); err != nil {
		t.Fatal(err)
	}
	if k.agent, err = c.Generate(); err != nil {
		t.Fatal(err)
	}
	k.ownerPub, _ = c.PublicKey(k.owner)
	k.agentPub, _ = c.PublicKey(k.agent)
	return k
}

func attested(t *testing.T, base string, k keys, conds string) identity.AgentIdentity {
	t.Helper()
	c := secp256k1.New()
	u, err := relay.ParseURL(base)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := attestation.ParseConditions(conds)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := c.Sign(k.owner, attestation.Message(k.agentPub, cs))
	if err != nil {
		t.Fatal(err)
	}
	id, err := identity.NewAttested(u, k.agent, k.agentPub, attestation.AuthTag{Owner: k.ownerPub, Conditions: cs, Sig: sig})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newClient(id identity.AgentIdentity) *buzzhttp.Client {
	return buzzhttp.New(id, secp256k1.New(),
		buzzhttp.WithClock(clocktest.At(now)),
		buzzhttp.WithNonce(func() (string, error) { return "fixed-nonce", nil }))
}

func TestQuery_AdmittedThroughOwnerAttestation(t *testing.T) {
	k := newKeys(t)
	r := fakerelay.Start(
		fakerelay.WithClock(clocktest.At(now)),
		fakerelay.WithNIPOA(true),
		fakerelay.WithMembers(k.ownerPub),
		fakerelay.WithChannels(fakerelay.Channel{ID: "c-1", Name: "teos-grok", Members: []key.PublicKey{k.agentPub}}),
	)
	defer r.Close()
	id := attested(t, r.URL(), k, "")

	evs, err := newClient(id).Query(context.Background(), relay.MetadataFilter(10))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 {
		t.Fatalf("events = %v", evs)
	}
	reqs := r.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d", len(reqs))
	}
	att, _ := id.Attestation()
	if reqs[0].AuthTag != att.Tag().String() {
		t.Fatalf("x-auth-tag = %q", reqs[0].AuthTag)
	}
	if reqs[0].Auth.PubKey != k.agentPub.Hex() || reqs[0].Auth.CreatedAt != now {
		t.Fatalf("NIP-98 event = %+v", reqs[0].Auth)
	}
	if n, _ := reqs[0].Auth.TagValue("nonce"); n != "fixed-nonce" {
		t.Fatalf("nonce = %q", n)
	}
}

func TestQuery_ClassifiesMembershipRefusal(t *testing.T) {
	k := newKeys(t)
	cases := map[string][]fakerelay.Option{
		"NIP-OA admission off":    {fakerelay.WithNIPOA(false), fakerelay.WithMembers(k.ownerPub)},
		"owner is not a member":   {fakerelay.WithNIPOA(true)},
		"attestation has expired": {fakerelay.WithNIPOA(true), fakerelay.WithMembers(k.ownerPub)},
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			r := fakerelay.Start(append(opts, fakerelay.WithClock(clocktest.At(now)))...)
			defer r.Close()
			conds := ""
			if name == "attestation has expired" {
				conds = "created_at<1700000000"
			}
			_, err := newClient(attested(t, r.URL(), k, conds)).Query(context.Background(), relay.MetadataFilter(1))
			if !shared.IsCode(err, shared.CodeRelayMembershipRequired) {
				t.Fatalf("want RELAY_MEMBERSHIP_REQUIRED, got %v", err)
			}
		})
	}
}

func TestQuery_MemberIdentitySendsNoAuthTag(t *testing.T) {
	k := newKeys(t)
	r := fakerelay.Start(fakerelay.WithClock(clocktest.At(now)), fakerelay.WithMembers(k.agentPub))
	defer r.Close()
	u, _ := relay.ParseURL(r.URL())
	id, err := identity.NewMember(u, k.agent, k.agentPub)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newClient(id).Query(context.Background(), relay.MetadataFilter(1)); err != nil {
		t.Fatal(err)
	}
	if tag := r.Requests()[0].AuthTag; tag != "" {
		t.Fatalf("member sent x-auth-tag %q", tag)
	}
}

func TestQuery_ClockSkewIsUnauthorized(t *testing.T) {
	k := newKeys(t)
	r := fakerelay.Start(fakerelay.WithClock(clocktest.At(now+3600)), fakerelay.WithMembers(k.agentPub))
	defer r.Close()
	u, _ := relay.ParseURL(r.URL())
	id, _ := identity.NewMember(u, k.agent, k.agentPub)
	_, err := newClient(id).Query(context.Background(), relay.MetadataFilter(1))
	if !shared.IsCode(err, shared.CodeRelayUnauthorized) {
		t.Fatalf("want RELAY_UNAUTHORIZED, got %v", err)
	}
}

func TestPost_ClassifiesStatusCodes(t *testing.T) {
	k := newKeys(t)
	cases := map[int]shared.Code{
		http.StatusBadRequest:          shared.CodeRelayRejected,
		http.StatusForbidden:           shared.CodeRelayRejected, // not a membership refusal
		http.StatusTooManyRequests:     shared.CodeRelayRejected,
		http.StatusInternalServerError: shared.CodeRelayUnavailable,
		http.StatusServiceUnavailable:  shared.CodeRelayUnavailable,
	}
	for status, want := range cases {
		r := fakerelay.Start(fakerelay.WithStatus(status))
		u, _ := relay.ParseURL(r.URL())
		id, _ := identity.NewMember(u, k.agent, k.agentPub)
		_, err := newClient(id).Query(context.Background(), relay.MetadataFilter(1))
		r.Close()
		if !shared.IsCode(err, want) {
			t.Errorf("HTTP %d: want %s, got %v", status, want, err)
		}
	}
}

func TestPost_UnreachableRelayIsUnavailable(t *testing.T) {
	k := newKeys(t)
	r := fakerelay.Start()
	u, _ := relay.ParseURL(r.URL())
	r.Close()
	id, _ := identity.NewMember(u, k.agent, k.agentPub)
	c := buzzhttp.New(id, secp256k1.New(), buzzhttp.WithHTTPClient(&http.Client{Timeout: time.Second}))
	if _, err := c.Query(context.Background(), relay.MetadataFilter(1)); !shared.IsCode(err, shared.CodeRelayUnavailable) {
		t.Fatalf("want RELAY_UNAVAILABLE, got %v", err)
	}
}

func TestPublish_ReturnsRelayVerdict(t *testing.T) {
	k := newKeys(t)
	r := fakerelay.Start(
		fakerelay.WithClock(clocktest.At(now)),
		fakerelay.WithNIPOA(true),
		fakerelay.WithMembers(k.ownerPub),
		fakerelay.WithChannels(fakerelay.Channel{ID: "c-1", Name: "teos-grok", Members: []key.PublicKey{k.agentPub}}),
	)
	defer r.Close()
	id := attested(t, r.URL(), k, "")
	c := newClient(id)
	crypto := secp256k1.New()

	sign := func(channel string) nostr.Event {
		u := nostr.NewUnsigned(k.agentPub, time.Unix(now, 0), nostr.KindChannelMessage, [][]string{{"h", channel}}, "hello")
		sig, err := crypto.Sign(k.agent, u.ID())
		if err != nil {
			t.Fatal(err)
		}
		return u.Signed(sig)
	}
	res, err := c.Publish(context.Background(), sign("c-1"))
	if err != nil || !res.Accepted || res.EventID == "" {
		t.Fatalf("publish = %+v, %v", res, err)
	}
	res, err = c.Publish(context.Background(), sign("c-2"))
	if err != nil || res.Accepted || res.Message == "" {
		t.Fatalf("publish to an invisible channel = %+v, %v", res, err)
	}
	if got := len(r.Messages()); got != 1 {
		t.Fatalf("relay stored %d messages", got)
	}
}
