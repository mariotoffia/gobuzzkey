package relay_test

import (
	"testing"

	"github.com/mariotoffia/gobuzzkey/domain/nostr"
	"github.com/mariotoffia/gobuzzkey/domain/relay"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

func TestParseURL_NormalizesToHTTPBase(t *testing.T) {
	cases := map[string]string{
		"wss://buzz.seven9.tech":  "https://buzz.seven9.tech",
		"wss://buzz.seven9.tech/": "https://buzz.seven9.tech",
		"WSS://Buzz.Seven9.tech":  "https://Buzz.Seven9.tech",
		"ws://localhost:3000":     "http://localhost:3000",
		" http://127.0.0.1:8080 ": "http://127.0.0.1:8080",
		"https://[::1]:443/":      "https://[::1]:443",
	}
	for in, want := range cases {
		u, err := relay.ParseURL(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if u.String() != want {
			t.Errorf("%q -> %q, want %q", in, u.String(), want)
		}
	}
}

func TestParseURL_RejectsUnsupported(t *testing.T) {
	for _, in := range []string{
		"", "buzz.seven9.tech", "ftp://relay.example", "https://", "https://relay.example/?x=1",
		"https://relay.example/#frag", "https://user:pw@relay.example",
		"https://relay.example/buzz",  // path prefix: Buzz signs scheme://host/route
		"https://relay.example//",     // still a path
		"https://relay.example?",      // forced empty query
		"https://a;id;.example",       // shell metacharacters in the host
		"https://relay.example:0",     // port out of range
		"https://relay.example:65536", // port out of range
		"https://relay.example:0443",  // non-canonical port
		"https://[zz::1]",             // bad IPv6 literal
	} {
		if _, err := relay.ParseURL(in); !shared.IsCode(err, shared.CodeInvalidRelayURL) {
			t.Errorf("%q: want INVALID_RELAY_URL, got %v", in, err)
		}
	}
}

func TestURL_Endpoint(t *testing.T) {
	u, err := relay.ParseURL("wss://relay.example/")
	if err != nil {
		t.Fatal(err)
	}
	if got := u.Endpoint("/query"); got != "https://relay.example/query" {
		t.Fatalf("endpoint = %s", got)
	}
}

func TestChannelFromMetadata(t *testing.T) {
	ev := nostr.Event{Kind: nostr.KindChannelMetadata, Tags: [][]string{{"d", "c-1"}, {"name", "teos-grok"}, {"about", "bots"}}}
	ch, ok := relay.ChannelFromMetadata(ev)
	if !ok || ch != (relay.Channel{ID: "c-1", Name: "teos-grok", About: "bots"}) {
		t.Fatalf("channel = %+v %v", ch, ok)
	}
	if _, ok := relay.ChannelFromMetadata(nostr.Event{Kind: nostr.KindChannelMessage, Tags: ev.Tags}); ok {
		t.Fatal("kind 9 read as metadata")
	}
	if _, ok := relay.ChannelFromMetadata(nostr.Event{Kind: nostr.KindChannelMetadata}); ok {
		t.Fatal("metadata without d tag accepted")
	}
}

func TestChannel_MatchesIDOrNameIgnoringCaseAndHash(t *testing.T) {
	ch := relay.Channel{ID: "c-1", Name: "teos-grok"}
	for _, ref := range []string{"c-1", "teos-grok", "TEOS-GROK", "#teos-grok", " teos-grok "} {
		if !ch.Matches(ref) {
			t.Errorf("%q did not match", ref)
		}
	}
	for _, ref := range []string{"", "#", "teos", "C-1"} {
		if ch.Matches(ref) {
			t.Errorf("%q matched", ref)
		}
	}
}

func TestChannel_GroupTagsAndFilters(t *testing.T) {
	ch := relay.Channel{ID: "c-1"}
	if tags := ch.GroupTags(); len(tags) != 1 || tags[0][0] != "h" || tags[0][1] != "c-1" {
		t.Fatalf("tags = %v", tags)
	}
	f := ch.HistoryFilter(5)
	if f.Limit != 5 || f.Kinds[0] != nostr.KindChannelMessage || f.Tags["h"][0] != "c-1" {
		t.Fatalf("filter = %+v", f)
	}
	m := ch.MembersFilter()
	if m.Limit != 1 || m.Kinds[0] != nostr.KindChannelMembers || m.Tags["d"][0] != "c-1" {
		t.Fatalf("members filter = %+v", m)
	}
}

func TestChannelFromMetadata_PrivateTagMarksAPrivateChannel(t *testing.T) {
	for tags, want := range map[string]bool{"private": true, "public": false, "": false} {
		ev := nostr.Event{Kind: nostr.KindChannelMetadata, Tags: [][]string{{"d", "c-1"}}}
		if tags != "" {
			ev.Tags = append(ev.Tags, []string{tags})
		}
		ch, _ := relay.ChannelFromMetadata(ev)
		if ch.Private != want {
			t.Errorf("%q: private = %v, want %v", tags, ch.Private, want)
		}
	}
}

func TestChannel_HasMemberReadsOnlyItsOwnMemberList(t *testing.T) {
	const agent = "aa"
	ch := relay.Channel{ID: "c-1"}
	list := func(d string, p ...string) nostr.Event {
		ev := nostr.Event{Kind: nostr.KindChannelMembers, Tags: [][]string{{"d", d}}}
		for _, pk := range p {
			ev.Tags = append(ev.Tags, []string{"p", pk, "member"})
		}
		return ev
	}
	cases := map[string]struct {
		events []nostr.Event
		want   bool
	}{
		"listed":                 {[]nostr.Event{list("c-1", "bb", agent)}, true},
		"not listed":             {[]nostr.Event{list("c-1", "bb")}, false},
		"listed in another":      {[]nostr.Event{list("c-2", agent)}, false},
		"wrong kind":             {[]nostr.Event{{Kind: nostr.KindChannelMetadata, Tags: [][]string{{"d", "c-1"}, {"p", agent}}}}, false},
		"no member list":         {nil, false},
		"pubkey only in d value": {[]nostr.Event{list(agent)}, false},
	}
	for name, tc := range cases {
		if got := ch.HasMember(tc.events, agent); got != tc.want {
			t.Errorf("%s: HasMember = %v, want %v", name, got, tc.want)
		}
	}
}
