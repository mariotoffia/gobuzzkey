package ports

import (
	"context"

	"github.com/mariotoffia/gobuzzkey/domain/nostr"
	"github.com/mariotoffia/gobuzzkey/domain/relay"
)

// Relay is an authenticated session with one Buzz relay. The adapter signs
// every request as the agent and presents its owner attestation; callers
// only see queries and writes.
//
// Errors carry a shared.Code: RELAY_MEMBERSHIP_REQUIRED, RELAY_UNAUTHORIZED,
// RELAY_REJECTED or RELAY_UNAVAILABLE.
type Relay interface {
	Query(ctx context.Context, filter nostr.Filter) ([]nostr.Event, error)
	Publish(ctx context.Context, ev nostr.Event) (relay.WriteResult, error)
}
