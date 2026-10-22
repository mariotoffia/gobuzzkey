package relay

import (
	"strings"

	"github.com/mariotoffia/gobuzzkey/domain/nostr"
)

// Channel is a Buzz channel (a NIP-29 group) as its kind-39000 metadata
// describes it.
//
// value-object
type Channel struct {
	ID      string // the "d" tag, a UUID on Buzz
	Name    string
	About   string
	Private bool // only members see it, and only a member can add another
}

// ChannelFromMetadata reads a kind-39000 event. It returns false for any
// other kind or an event without a "d" tag.
func ChannelFromMetadata(ev nostr.Event) (Channel, bool) {
	if ev.Kind != nostr.KindChannelMetadata {
		return Channel{}, false
	}
	id, ok := ev.TagValue("d")
	if !ok || id == "" {
		return Channel{}, false
	}
	name, _ := ev.TagValue("name")
	about, _ := ev.TagValue("about")
	return Channel{ID: id, Name: name, About: about, Private: hasFlag(ev, "private")}, true
}

// hasFlag reports whether ev carries a one-element tag such as ["private"].
func hasFlag(ev nostr.Event, name string) bool {
	for _, t := range ev.Tags {
		if len(t) > 0 && t[0] == name {
			return true
		}
	}
	return false
}

// Matches reports whether ref names this channel by ID or, case-insensitively,
// by name. A leading '#' is ignored.
func (c Channel) Matches(ref string) bool {
	ref = strings.TrimPrefix(strings.TrimSpace(ref), "#")
	return ref != "" && (ref == c.ID || strings.EqualFold(ref, c.Name))
}

// GroupTags returns the "h" tag that addresses the channel in a message, a
// join request or a leave request.
func (c Channel) GroupTags() [][]string {
	return [][]string{{"h", c.ID}}
}

// HistoryFilter returns a filter for the channel's recent messages.
func (c Channel) HistoryFilter(limit int) nostr.Filter {
	return nostr.Filter{
		Kinds: []int{nostr.KindChannelMessage},
		Tags:  map[string][]string{"h": {c.ID}},
		Limit: limit,
	}
}

// MembersFilter returns a filter for the channel's member list.
func (c Channel) MembersFilter() nostr.Filter {
	return nostr.Filter{
		Kinds: []int{nostr.KindChannelMembers},
		Tags:  map[string][]string{"d": {c.ID}},
		Limit: 1,
	}
}

// HasMember reports whether pubHex is a "p" tag of the channel's member list
// among events. Lists of other channels and other kinds are ignored.
func (c Channel) HasMember(events []nostr.Event, pubHex string) bool {
	for _, ev := range events {
		if d, _ := ev.TagValue("d"); ev.Kind != nostr.KindChannelMembers || d != c.ID {
			continue
		}
		for _, t := range ev.Tags {
			if len(t) >= 2 && t[0] == "p" && t[1] == pubHex {
				return true
			}
		}
	}
	return false
}

// MetadataFilter returns a filter for every channel the caller can see.
func MetadataFilter(limit int) nostr.Filter {
	return nostr.Filter{Kinds: []int{nostr.KindChannelMetadata}, Limit: limit}
}

// WriteResult is the relay's answer to a published event.
//
// value-object
type WriteResult struct {
	EventID  string
	Accepted bool
	Message  string
}
