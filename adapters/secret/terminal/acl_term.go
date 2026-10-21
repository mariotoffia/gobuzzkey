package terminal

// Anti-corruption layer: the only file that imports golang.org/x/term.

import (
	"context"
	"fmt"

	"golang.org/x/term"
)

func isTerminal(fd uintptr) bool { return term.IsTerminal(int(fd)) }

// readHidden reads one line without echo. If ctx ends first (Ctrl-C: the
// terminal keeps ISIG, so the signal reaches the process), the terminal state
// is restored before returning, so the user's shell does not stay silent.
func readHidden(ctx context.Context, fd uintptr) ([]byte, error) {
	state, err := term.GetState(int(fd))
	if err != nil {
		return nil, fmt.Errorf("terminal state: %w", err)
	}
	type result struct {
		b   []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		b, err := term.ReadPassword(int(fd))
		done <- result{b, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			return nil, fmt.Errorf("read password: %w", r.err)
		}
		return r.b, nil
	case <-ctx.Done():
		_ = term.Restore(int(fd), state)
		return nil, fmt.Errorf("read password: %w", ctx.Err())
	}
}
