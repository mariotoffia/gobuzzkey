package app

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mariotoffia/gobuzzkey/domain/clock"
	"github.com/mariotoffia/gobuzzkey/domain/identity"
	"github.com/mariotoffia/gobuzzkey/domain/nostr"
	"github.com/mariotoffia/gobuzzkey/domain/relay"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
	"github.com/mariotoffia/gobuzzkey/ports"
)

// Query sizes. The metadata limit matches Buzz's max page size.
const (
	channelLimit       = 1000
	defaultHistorySize = 3
)

// Checker proves an identity works against a live relay.
type Checker struct {
	signer ports.Signer
	clock  clock.Clock
}

// NewChecker wires the check use case.
func NewChecker(signer ports.Signer, clk clock.Clock) *Checker {
	return &Checker{signer: signer, clock: clk}
}

// CheckRequest selects how far the check goes.
type CheckRequest struct {
	// Channel, when set, is looked up by ID or name and read.
	Channel string
	// Send, when set with Channel, is posted to it as the agent.
	Send string
	// History is how many recent messages to read; 0 means 3.
	History int
}

// CheckReport is what the relay showed the agent.
type CheckReport struct {
	Admitted bool            // the relay answered the agent's first query
	Channels []relay.Channel // sorted by name
	Channel  relay.Channel   // zero unless CheckRequest.Channel matched
	Recent   int             // messages read from Channel
	Posted   relay.WriteResult
}

// Check lists the channels the agent can see, then optionally reads and posts
// to one. An expired attestation is reported before any network call.
func (c *Checker) Check(ctx context.Context, id identity.AgentIdentity, r ports.Relay, req CheckRequest) (CheckReport, error) {
	if att, ok := id.Attestation(); ok && !att.AdmittedAt(c.clock.Now()) {
		exp, _ := att.Expiry()
		return CheckReport{}, shared.New(shared.CodeAttestationExpired,
			"attestation expired at "+exp.Format("2006-01-02T15:04:05Z07:00"))
	}

	metas, err := r.Query(ctx, relay.MetadataFilter(channelLimit))
	if err != nil {
		return CheckReport{}, err
	}
	report := CheckReport{Admitted: true, Channels: channelsFrom(metas)}
	if req.Channel == "" {
		return report, nil
	}
	if req.Send != "" && !utf8.ValidString(req.Send) {
		return report, shared.New(shared.CodeInvalidEvent, "message text is not valid UTF-8")
	}
	ch, err := findChannel(report.Channels, req.Channel, id)
	if err != nil {
		return report, err
	}
	report.Channel = ch

	limit := req.History
	if limit <= 0 {
		limit = defaultHistorySize
	}
	msgs, err := r.Query(ctx, ch.HistoryFilter(limit))
	if err != nil {
		return report, err
	}
	report.Recent = len(msgs)

	if req.Send == "" {
		return report, nil
	}
	report.Posted, err = c.post(ctx, id, r, ch, req.Send)
	return report, err
}

// channelsFrom reads kind-39000 events into channels sorted by name, then ID,
// so the order is stable when names repeat.
func channelsFrom(metas []nostr.Event) []relay.Channel {
	var channels []relay.Channel
	for _, ev := range metas {
		if ch, ok := relay.ChannelFromMetadata(ev); ok {
			channels = append(channels, ch)
		}
	}
	sort.Slice(channels, func(a, b int) bool {
		x, y := channels[a], channels[b]
		if x.Name != y.Name {
			return x.Name < y.Name
		}
		return x.ID < y.ID
	})
	return channels
}

// post signs a kind-9 message to ch and publishes it.
func (c *Checker) post(ctx context.Context, id identity.AgentIdentity, r ports.Relay, ch relay.Channel, text string) (relay.WriteResult, error) {
	ev, err := c.sign(id, nostr.KindChannelMessage, ch.MessageTags(), text)
	if err != nil {
		return relay.WriteResult{}, err
	}
	res, err := r.Publish(ctx, ev)
	if err != nil {
		return relay.WriteResult{}, err
	}
	if !res.Accepted {
		return res, shared.New(shared.CodeRelayRejected, "relay did not accept the message: "+res.Message)
	}
	return res, nil
}

// sign builds and signs an event as the agent. An attested agent includes its
// auth tag whenever the attestation covers the event, as buzz-cli does, so
// clients can show "authorized by <owner>" (NIP-OA, client behaviour).
func (c *Checker) sign(id identity.AgentIdentity, kind int, tags [][]string, content string) (nostr.Event, error) {
	now := c.clock.Now()
	if att, ok := id.Attestation(); ok && att.Tag().Conditions.Allows(kind, now.Unix()) {
		tags = append(append([][]string(nil), tags...), att.Tag().Strings())
	}
	u := nostr.NewUnsigned(id.Agent(), now, kind, tags, content)
	sig, err := c.signer.Sign(id.Secret(), u.ID())
	if err != nil {
		return nostr.Event{}, err
	}
	return u.Signed(sig), nil
}

// findChannel resolves ref by exact ID first, then by name. Buzz does not
// require channel names to be unique, so a name that matches several channels
// is an error rather than a guess.
func findChannel(channels []relay.Channel, ref string, id identity.AgentIdentity) (relay.Channel, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(ref), "#")
	var matches []relay.Channel
	for _, ch := range channels {
		if ch.ID == trimmed {
			return ch, nil
		}
		if ch.Matches(ref) {
			matches = append(matches, ch)
		}
	}
	switch len(matches) {
	case 0:
		return relay.Channel{}, shared.New(shared.CodeChannelNotVisible,
			"channel "+strconv.Quote(ref)+" is not visible to agent "+id.Agent().Npub())
	case 1:
		return matches[0], nil
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return relay.Channel{}, shared.New(shared.CodeChannelAmbiguous,
			strconv.Itoa(len(matches))+" channels are named "+strconv.Quote(ref)+"; pass one of these IDs instead: "+strings.Join(ids, ", "))
	}
}
