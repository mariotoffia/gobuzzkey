package clock

import "time"

// Real is the wall clock. It is the only production code allowed to call
// time.Now (.golangci.yml forbidigo exemption).
type Real struct{}

// Now returns the current wall-clock time.
func (Real) Now() time.Time { return time.Now() }

var _ Clock = Real{}
