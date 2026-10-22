package app

import (
	"context"
	"strconv"

	"github.com/mariotoffia/gobuzzkey/domain/identity"
	"github.com/mariotoffia/gobuzzkey/domain/nostr"
	"github.com/mariotoffia/gobuzzkey/domain/relay"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
	"github.com/mariotoffia/gobuzzkey/ports"
)

// MembershipReport says what Join or Leave did to the agent's channel
// membership.
type MembershipReport struct {
	Channel relay.Channel
	Changed bool // false when the agent already was (Join) or was not (Leave) a member
}

// Join makes the agent a member of an open channel with a join request signed
// by the agent itself, so the owner key is not needed. A private channel is
// CHANNEL_NOT_VISIBLE unless the agent already is a member: Buzz only lets a
// member add someone to it. Joining twice publishes nothing.
func (c *Checker) Join(ctx context.Context, id identity.AgentIdentity, r ports.Relay, ref string) (MembershipReport, error) {
	ch, member, err := c.membership(ctx, id, r, ref)
	if err != nil || member {
		return MembershipReport{Channel: ch}, err
	}
	if ch.Private {
		return MembershipReport{Channel: ch}, shared.New(shared.CodeChannelNotVisible,
			"channel "+strconv.Quote(ref)+" is private; a channel member has to add agent "+id.Agent().Npub())
	}
	if _, err := c.publish(ctx, id, r, nostr.KindJoinRequest, ch, ""); err != nil {
		return MembershipReport{Channel: ch}, err
	}
	return MembershipReport{Channel: ch, Changed: true}, nil
}

// Leave takes the agent out of a channel it is a member of. Leaving a channel
// the agent is not in publishes nothing.
func (c *Checker) Leave(ctx context.Context, id identity.AgentIdentity, r ports.Relay, ref string) (MembershipReport, error) {
	ch, member, err := c.membership(ctx, id, r, ref)
	if err != nil || !member {
		return MembershipReport{Channel: ch}, err
	}
	if _, err := c.publish(ctx, id, r, nostr.KindLeaveRequest, ch, ""); err != nil {
		return MembershipReport{Channel: ch}, err
	}
	return MembershipReport{Channel: ch, Changed: true}, nil
}

// membership resolves ref among the channels the agent can see and reads the
// channel's member list.
func (c *Checker) membership(ctx context.Context, id identity.AgentIdentity, r ports.Relay, ref string) (relay.Channel, bool, error) {
	if err := c.admissible(id); err != nil {
		return relay.Channel{}, false, err
	}
	metas, err := r.Query(ctx, relay.MetadataFilter(channelLimit))
	if err != nil {
		return relay.Channel{}, false, err
	}
	ch, err := findChannel(channelsFrom(metas), ref, id)
	if err != nil {
		return relay.Channel{}, false, err
	}
	lists, err := r.Query(ctx, ch.MembersFilter())
	if err != nil {
		return ch, false, err
	}
	return ch, ch.HasMember(lists, id.Agent().Hex()), nil
}
