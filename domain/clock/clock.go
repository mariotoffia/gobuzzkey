// Package clock is the time abstraction. Production code never reads the wall
// clock directly; it receives a Clock so tests can pin time.
package clock

import "time"

// Clock reports the current time.
type Clock interface {
	Now() time.Time
}
