package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/mariotoffia/gobuzzkey/adapters/crypto/secp256k1"
	"github.com/mariotoffia/gobuzzkey/adapters/secret/terminal"
	"github.com/mariotoffia/gobuzzkey/adapters/store/envfile"
	"github.com/mariotoffia/gobuzzkey/app"
	"github.com/mariotoffia/gobuzzkey/domain/identity"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// envOwnerSecret, when set, supplies the owner secret instead of the prompt.
const envOwnerSecret = "BUZZ_OWNER_PRIVATE_KEY"

const defaultIdentityPath = "buzz-agent.env"

// defaultExpiry bounds a new attestation, which cannot be withdrawn.
const defaultExpiry = "3y"

func cmdNew(ctx context.Context, e env) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	relayURL := fs.String("relay", "", "Buzz relay URL (ws, wss, http or https)")
	channel := fs.String("channel", "", "channel name or ID the agent joins and reads")
	out := fs.String("out", defaultIdentityPath, `identity file; kept when it exists. "-" writes it to stdout, without the Grok Bot prompt`)
	format := fs.String("format", string(envfile.FormatEnv), "identity format: env or json")
	expires := fs.String("expires", defaultExpiry, "expiry of a new identity: YYYY-MM-DD (00:00 UTC), RFC 3339, or a duration such as 3y, 90d or 36h")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(e.stderr, "Usage: gobuzzkey new -relay <url> [-channel <name|id>] [flags]\n\n"+
			"Sets up an agent identity in one go and is safe to run again:\n"+
			"  1. creates the identity in -out, or keeps the one already there. A new one\n"+
			"     needs your (the owner's) key, read from a hidden prompt, piped stdin or\n"+
			"     $%s, used once and never stored;\n"+
			"  2. checks that the relay admits the agent;\n"+
			"  3. joins -channel (open channels only) and reads it;\n"+
			"  4. prints the prompt to paste into a Grok Bot chat.\n\nFlags:\n", envOwnerSecret)
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, e.args); err != nil {
		return err
	}
	if *relayURL == "" || fs.NArg() > 0 {
		fs.Usage()
		return errUsage
	}
	if f := envfile.Format(*format); f != envfile.FormatEnv && f != envfile.FormatJSON {
		_, _ = fmt.Fprintf(e.stderr, "-format must be env or json, got %q\n", *format)
		return errUsage
	}
	until, err := parseExpiry(*expires, e.clock.Now())
	if err != nil {
		return err
	}

	w := e.stdout
	if *out == envfile.Stdout {
		w = e.stderr
	}
	store, err := envfile.New(*out, envfile.Format(*format), envfile.WithStdout(e.stdout))
	if err != nil {
		return err
	}
	crypto := secp256k1.New()
	id, err := openOrMint(ctx, e, w, store, crypto, *out, app.MintRequest{RelayURL: *relayURL, Until: until})
	if err != nil {
		return err
	}
	err = enroll(ctx, w, crypto, e.clock, id, *channel)
	printNext(w, id, *channel, err)
	// With "-out -" the identity itself is on stdout, headed for a pipe.
	if *out != envfile.Stdout {
		if perr := printGrok(ctx, w, id, err); perr != nil {
			return perr
		}
	}
	if err != nil {
		return explained{err}
	}
	return nil
}

// openOrMint keeps the identity at out when there is one for the same relay,
// so a second run neither asks for the owner key nor replaces the agent.
func openOrMint(ctx context.Context, e env, w io.Writer, store *envfile.Store, crypto secp256k1.Crypto, out string, req app.MintRequest) (identity.AgentIdentity, error) {
	// Writable reports an existing file as IDENTITY_EXISTS; that file is reused.
	if err := store.Writable(ctx); !shared.IsCode(err, shared.CodeIdentityExists) {
		if err != nil {
			return identity.AgentIdentity{}, err
		}
		prompt := terminal.New(
			terminal.WithPreset(e.getenv(envOwnerSecret)),
			terminal.WithInput(e.stdin),
			terminal.WithOutput(e.stderr),
		)
		res, err := app.NewMinter(crypto, crypto, crypto, prompt, store, e.clock).Mint(ctx, req)
		if err != nil {
			return identity.AgentIdentity{}, err
		}
		printMinted(w, out, res)
		return res.Identity, nil
	}
	ins := app.NewInspector(crypto, crypto, e.clock)
	id, err := ins.Open(ctx, store)
	if err != nil {
		return identity.AgentIdentity{}, err
	}
	if err := ins.Reusable(id, req.RelayURL); err != nil {
		return identity.AgentIdentity{}, err
	}
	step(w, nil, "identity", "keeping "+out+expirySuffix(id))
	return id, nil
}

func printMinted(w io.Writer, out string, res app.MintResult) {
	id := res.Identity
	if out == envfile.Stdout {
		step(w, nil, "identity", "written to stdout")
	} else {
		step(w, nil, "identity", "created "+out+" (mode 0600)")
	}
	_, _ = fmt.Fprintf(w, "%16sagent    %s\n", "", id.Agent().Npub())
	_, _ = fmt.Fprintf(w, "%16sowner    %s  (must be a member of the relay)\n", "", res.Owner.Npub())
	_, _ = fmt.Fprintf(w, "%16srelay    %s\n", "", id.Relay())
	if att, ok := id.Attestation(); ok {
		if exp, ok := att.Expiry(); ok {
			_, _ = fmt.Fprintf(w, "%16sexpires  %s\n", "", exp.Format(time.RFC3339))
		}
	}
}

// expirySuffix describes when an identity stops being admitted.
func expirySuffix(id identity.AgentIdentity) string {
	if att, ok := id.Attestation(); ok {
		if exp, ok := att.Expiry(); ok {
			return " (expires " + exp.Format(time.RFC3339) + ")"
		}
	}
	return ""
}

// maxExpiry is the largest created_at a NIP-OA clause can hold (2106-02-07).
const maxExpiry = 4294967295

// maxExpiryDays and maxExpiryYears keep the date arithmetic far from overflow.
const (
	maxExpiryDays  = 36500
	maxExpiryYears = 100
)

// parseExpiry accepts "", a date (00:00 UTC), RFC 3339, or a positive duration
// with an optional "y" (years) or "d" (days) unit. The result must fit a NIP-OA
// clause.
func parseExpiry(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	t, ok := parseWhen(s, now)
	if !ok {
		return time.Time{}, shared.New(shared.CodeInvalidConditions,
			"-expires "+strconv.Quote(s)+" is not a date, an RFC 3339 time or a positive duration such as 3y, 90d or 36h")
	}
	if t.Unix() > maxExpiry {
		return time.Time{}, shared.New(shared.CodeInvalidConditions,
			"-expires "+strconv.Quote(s)+" is after "+time.Unix(maxExpiry, 0).UTC().Format(time.RFC3339)+
				", the latest time a NIP-OA condition can express")
	}
	return t, nil
}

func parseWhen(s string, now time.Time) (time.Time, bool) {
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), true
	}
	if years, ok := strings.CutSuffix(s, "y"); ok {
		n, err := strconv.Atoi(years)
		if err != nil || n <= 0 || n > maxExpiryYears {
			return time.Time{}, false
		}
		return now.AddDate(n, 0, 0).UTC(), true
	}
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 || n > maxExpiryDays {
			return time.Time{}, false
		}
		return now.AddDate(0, 0, n).UTC(), true
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return now.Add(d).UTC(), true
	}
	return time.Time{}, false
}
