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
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// envOwnerSecret, when set, supplies the owner secret instead of the prompt.
const envOwnerSecret = "BUZZ_OWNER_PRIVATE_KEY"

const defaultIdentityPath = "buzz-agent.env"

func cmdNew(ctx context.Context, e env) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	relayURL := fs.String("relay", "", "Buzz relay URL (ws, wss, http or https)")
	out := fs.String("out", defaultIdentityPath, `where to write the identity; "-" writes to stdout`)
	format := fs.String("format", string(envfile.FormatEnv), "identity format: env or json")
	expires := fs.String("expires", "", "optional expiry: YYYY-MM-DD (00:00 UTC), RFC 3339, or a duration such as 90d or 36h")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(e.stderr, "Usage: gobuzzkey new -relay <url> [flags]\n\n"+
			"Generates an agent key, signs a NIP-OA attestation for it with your (the owner's)\n"+
			"key and writes BUZZ_RELAY_URL, BUZZ_PRIVATE_KEY and BUZZ_AUTH_TAG. Your key is read\n"+
			"from a hidden prompt, piped stdin or $%s, used once and never stored.\n\nFlags:\n", envOwnerSecret)
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

	statusOut := e.stdout
	if *out == envfile.Stdout {
		statusOut = e.stderr
	}
	store, err := envfile.New(*out, envfile.Format(*format), envfile.WithStdout(e.stdout))
	if err != nil {
		return err
	}
	crypto := secp256k1.New()
	prompt := terminal.New(
		terminal.WithPreset(e.getenv(envOwnerSecret)),
		terminal.WithInput(e.stdin),
		terminal.WithOutput(e.stderr),
	)
	minter := app.NewMinter(crypto, crypto, crypto, prompt, store, e.clock)

	res, err := minter.Mint(ctx, app.MintRequest{RelayURL: *relayURL, Until: until})
	if err != nil {
		return err
	}
	printMinted(statusOut, *out, res)
	return nil
}

func printMinted(w io.Writer, out string, res app.MintResult) {
	id := res.Identity
	if out == envfile.Stdout {
		_, _ = fmt.Fprintln(w, "identity written to stdout")
	} else {
		_, _ = fmt.Fprintf(w, "wrote %s (mode 0600)\n", out)
	}
	_, _ = fmt.Fprintf(w, "  agent    %s\n", id.Agent().Npub())
	_, _ = fmt.Fprintf(w, "  owner    %s  (must be a member of the relay)\n", res.Owner.Npub())
	_, _ = fmt.Fprintf(w, "  relay    %s\n", id.Relay())
	if att, ok := id.Attestation(); ok {
		if exp, ok := att.Expiry(); ok {
			_, _ = fmt.Fprintf(w, "  expires  %s\n", exp.Format(time.RFC3339))
		}
	}
	checkPath := out
	if out == envfile.Stdout {
		checkPath = "<file>"
	}
	_, _ = fmt.Fprintf(w, "\nNext: add the agent npub to your channel in Buzz, then run\n"+
		"  gobuzzkey check -env %s -channel <name>\n", checkPath)
}

// maxExpiry is the largest created_at a NIP-OA clause can hold (2106-02-07).
const maxExpiry = 4294967295

// maxExpiryDays keeps the day arithmetic far from overflow.
const maxExpiryDays = 36500

// parseExpiry accepts "", a date (00:00 UTC), RFC 3339, or a positive duration
// with an optional "d" (days) unit. The result must fit a NIP-OA clause.
func parseExpiry(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	t, ok := parseWhen(s, now)
	if !ok {
		return time.Time{}, shared.New(shared.CodeInvalidConditions,
			"-expires "+strconv.Quote(s)+" is not a date, an RFC 3339 time or a positive duration of at most "+
				strconv.Itoa(maxExpiryDays)+"d")
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
