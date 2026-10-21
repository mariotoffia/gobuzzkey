// Package terminal reads the owner's secret key: from a preset value, from a
// hidden prompt on a terminal, or from one line of piped input.
package terminal

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
	"github.com/mariotoffia/gobuzzkey/ports"
)

const promptText = "Owner secret key (nsec1… or hex, input hidden): "

// Prompt implements ports.SecretPrompt.
type Prompt struct {
	preset string
	in     *os.File
	out    io.Writer
}

// Option configures a Prompt.
type Option func(*Prompt)

// WithPreset supplies the secret directly, e.g. from an environment variable
// the composition root read. An empty preset is ignored.
func WithPreset(secret string) Option { return func(p *Prompt) { p.preset = secret } }

// WithInput sets the input file (default os.Stdin).
func WithInput(f *os.File) Option { return func(p *Prompt) { p.in = f } }

// WithOutput sets where the prompt is written (default os.Stderr).
func WithOutput(w io.Writer) Option { return func(p *Prompt) { p.out = w } }

// New returns a prompt.
func New(opts ...Option) *Prompt {
	p := &Prompt{in: os.Stdin, out: os.Stderr}
	for _, o := range opts {
		o(p)
	}
	return p
}

// OwnerSecret returns the parsed owner secret key. It returns early, with an
// error wrapping ctx.Err(), when ctx ends while waiting for input.
func (p *Prompt) OwnerSecret(ctx context.Context) (key.SecretKey, error) {
	raw, err := p.read(ctx)
	if err != nil {
		return key.SecretKey{}, err
	}
	if raw == "" {
		return key.SecretKey{}, shared.New(shared.CodeSecretUnavailable, "no owner secret key given")
	}
	sk, err := key.ParseSecretKey(raw)
	if err != nil {
		return key.SecretKey{}, shared.Wrap(shared.CodeSecretUnavailable, "owner secret key", err)
	}
	return sk, nil
}

func (p *Prompt) read(ctx context.Context) (string, error) {
	if p.preset != "" {
		return strings.TrimSpace(p.preset), nil
	}
	if isTerminal(p.in.Fd()) {
		_, _ = io.WriteString(p.out, promptText)
		b, err := readHidden(ctx, p.in.Fd())
		_, _ = io.WriteString(p.out, "\n")
		if err != nil {
			return "", shared.Wrap(shared.CodeSecretUnavailable, "read hidden input", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return readLine(ctx, p.in)
}

// readLine reads one line from a pipe or file, giving up when ctx ends.
func readLine(ctx context.Context, in io.Reader) (string, error) {
	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := bufio.NewReader(in).ReadString('\n')
		done <- result{line, err}
	}()
	select {
	case r := <-done:
		if r.err != nil && !errors.Is(r.err, io.EOF) {
			return "", shared.Wrap(shared.CodeSecretUnavailable, "read input", r.err)
		}
		return strings.TrimSpace(r.line), nil
	case <-ctx.Done():
		return "", shared.Wrap(shared.CodeSecretUnavailable, "interrupted while reading input", ctx.Err())
	}
}

var _ ports.SecretPrompt = (*Prompt)(nil)
