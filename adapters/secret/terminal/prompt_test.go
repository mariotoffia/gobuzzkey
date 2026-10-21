package terminal_test

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/mariotoffia/gobuzzkey/adapters/secret/terminal"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

const nsec = "nsec1vl029mgpspedva04g90vltkh6fvh240zqtv9k0t9af8935ke9laqsnlfe5"

// pipe returns a read end preloaded with input; a pipe is never a terminal.
func pipe(t *testing.T, input string) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if _, err := io.WriteString(w, input); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestOwnerSecret_PresetWins(t *testing.T) {
	p := terminal.New(terminal.WithPreset(" "+nsec+" "), terminal.WithInput(pipe(t, "ignored\n")))
	sk, err := p.OwnerSecret(context.Background())
	if err != nil || sk.Nsec() != nsec {
		t.Fatalf("secret = %v, %v", sk.Nsec(), err)
	}
}

func TestOwnerSecret_ReadsOnePipedLine(t *testing.T) {
	p := terminal.New(terminal.WithInput(pipe(t, nsec+"\nsecond line\n")), terminal.WithOutput(io.Discard))
	sk, err := p.OwnerSecret(context.Background())
	if err != nil || sk.Nsec() != nsec {
		t.Fatalf("secret = %v, %v", sk.Nsec(), err)
	}
}

func TestOwnerSecret_MissingOrInvalidIsUnavailable(t *testing.T) {
	for name, input := range map[string]string{"empty": "", "blank line": "\n", "not a key": "hello\n"} {
		t.Run(name, func(t *testing.T) {
			p := terminal.New(terminal.WithInput(pipe(t, input)))
			if _, err := p.OwnerSecret(context.Background()); !shared.IsCode(err, shared.CodeSecretUnavailable) {
				t.Fatalf("want SECRET_UNAVAILABLE, got %v", err)
			}
		})
	}
}

func TestOwnerSecret_ReturnsWhenTheContextEnds(t *testing.T) {
	r, w, err := os.Pipe() // nothing is ever written: the read blocks
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = terminal.New(terminal.WithInput(r)).OwnerSecret(ctx)
	if !errors.Is(err, context.Canceled) || !shared.IsCode(err, shared.CodeSecretUnavailable) {
		t.Fatalf("want SECRET_UNAVAILABLE wrapping context.Canceled, got %v", err)
	}
}
