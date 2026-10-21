package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/mariotoffia/gobuzzkey/adapters/crypto/secp256k1"
	"github.com/mariotoffia/gobuzzkey/adapters/store/envfile"
	"github.com/mariotoffia/gobuzzkey/app"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

func cmdVerify(ctx context.Context, e env) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	envPath := fs.String("env", defaultIdentityPath, "identity file (env or JSON) to verify")
	tag := fs.String("tag", "", "verify this auth tag JSON instead of a file (needs -agent)")
	agent := fs.String("agent", "", "agent public key (npub or hex) the -tag was issued for")
	fs.Usage = func() {
		_, _ = fmt.Fprint(e.stderr, "Usage: gobuzzkey verify [-env <file>] | -tag <json> -agent <npub|hex>\n\n"+
			"Checks offline that the secret key matches the attested agent, that the owner\n"+
			"signature verifies and whether the attestation has expired.\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, e.args); err != nil {
		return err
	}
	if fs.NArg() > 0 || (*tag == "") != (*agent == "") {
		fs.Usage()
		return errUsage
	}

	crypto := secp256k1.New()
	inspector := app.NewInspector(crypto, crypto, e.clock)

	var rep app.Report
	if *tag != "" {
		r, err := inspector.VerifyTag(*tag, *agent)
		if err != nil {
			return err
		}
		rep = r
	} else {
		store, err := envfile.New(*envPath, envfile.FormatEnv)
		if err != nil {
			return err
		}
		id, err := inspector.Open(ctx, store)
		if err != nil {
			return err
		}
		rep = inspector.Inspect(id)
	}
	printReport(e.stdout, rep)
	if rep.Expired {
		return shared.New(shared.CodeAttestationExpired, "the relay would refuse this attestation now")
	}
	return nil
}

func printReport(w io.Writer, r app.Report) {
	if !r.Relay.IsZero() {
		_, _ = fmt.Fprintf(w, "relay       %s\n", r.Relay)
	}
	_, _ = fmt.Fprintf(w, "agent       %s\n", r.Agent.Npub())
	if !r.Attested {
		_, _ = fmt.Fprintln(w, "attestation none (the agent must be a relay member itself)")
		return
	}
	_, _ = fmt.Fprintf(w, "owner       %s\n", r.Owner.Npub())
	conds := r.Conditions
	if conds == "" {
		conds = "(none)"
	}
	_, _ = fmt.Fprintf(w, "conditions  %s\n", conds)
	if r.Expiry.IsZero() {
		_, _ = fmt.Fprintln(w, "expires     never")
	} else {
		_, _ = fmt.Fprintf(w, "expires     %s\n", r.Expiry.Format(time.RFC3339))
	}
	_, _ = fmt.Fprintln(w, "signature   valid")
	if r.Expired {
		_, _ = fmt.Fprintln(w, "status      EXPIRED")
	} else {
		_, _ = fmt.Fprintln(w, "status      admissible now")
	}
}
