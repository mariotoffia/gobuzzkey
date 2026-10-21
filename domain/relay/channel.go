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
	ID    string // the "d" tag, a UUID on Buzz
	Name  string
	About string
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
	return Channel{ID: id, Name: name, About: about}, true
}

// Matches reports whether ref names this channel by ID or, case-insensitively,
// by name. A leading '#' is ignored.
func (c Channel) Matches(ref string) bool {
	ref = strings.TrimPrefix(strings.TrimSpace(ref), "#")
	return ref != "" && (ref == c.ID || strings.EqualFold(ref, c.Name))
}

// MessageTags returns the tags of a kind-9 message posted to the channel.
func (c Channel) MessageTags() [][]string {
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
