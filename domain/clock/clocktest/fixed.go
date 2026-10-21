// Package clocktest provides a clock that always reports the same instant.
package clocktest

import (
	"time"

	"github.com/mariotoffia/gobuzzkey/domain/clock"
)

// Fixed reports T on every call.
type Fixed struct{ T time.Time }

// Now returns T.
func (f Fixed) Now() time.Time { return f.T }

// At returns a Fixed clock at the given unix second.
func At(unix int64) Fixed { return Fixed{T: time.Unix(unix, 0).UTC()} }

var _ clock.Clock = Fixed{}
