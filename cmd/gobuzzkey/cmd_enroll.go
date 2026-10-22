package main

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/mariotoffia/gobuzzkey/adapters/crypto/secp256k1"
	"github.com/mariotoffia/gobuzzkey/adapters/relay/buzzhttp"
	"github.com/mariotoffia/gobuzzkey/adapters/store/envfile"
	"github.com/mariotoffia/gobuzzkey/app"
	"github.com/mariotoffia/gobuzzkey/domain/clock"
	"github.com/mariotoffia/gobuzzkey/domain/identity"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// step prints one progress line: "ok" or "FAIL", the step name and what
// happened. Relay text in err is made printable.
func step(w io.Writer, err error, name, text string) {
	status := "ok  "
	if err != nil {
		status = "FAIL"
		text = printable(err.Error())
	}
	_, _ = fmt.Fprintf(w, "%s  %-8s  %s\n", status, name, text)
}

// enroll proves the relay admits the agent and, when channel is set, joins it
// and reads it. It stops at the first failing step.
func enroll(ctx context.Context, w io.Writer, crypto secp256k1.Crypto, clk clock.Clock, id identity.AgentIdentity, channel string) error {
	client := buzzhttp.New(id, crypto, buzzhttp.WithClock(clk))
	checker := app.NewChecker(crypto, clk)
	if _, err := checker.Check(ctx, id, client, app.CheckRequest{}); err != nil {
		step(w, err, "relay", "")
		return err
	}
	step(w, nil, "relay", id.Relay().String()+" admits the agent")
	if channel == "" {
		return nil
	}
	m, err := checker.Join(ctx, id, client, channel)
	if err != nil {
		step(w, err, "channel", "")
		return err
	}
	name := printable(m.Channel.Name) + " (" + printable(m.Channel.ID) + ")"
	if m.Changed {
		step(w, nil, "channel", "joined "+name)
	} else {
		step(w, nil, "channel", "already a member of "+name)
	}
	rep, err := checker.Check(ctx, id, client, app.CheckRequest{Channel: m.Channel.ID})
	if err != nil {
		step(w, err, "check", "")
		return err
	}
	step(w, nil, "check", "read "+strconv.Itoa(rep.Recent)+" recent message(s) in "+printable(m.Channel.Name))
	return nil
}

// printNext prints what is left to do after the steps: for the relay admin or
// for the owner in Buzz Desktop.
func printNext(w io.Writer, id identity.AgentIdentity, channel string, err error) {
	agent := id.Agent().Npub()
	switch {
	case shared.IsCode(err, shared.CodeRelayMembershipRequired):
		_, _ = fmt.Fprintf(w, "\nAsk the admin of %s to do one of these, then run this command again:\n", id.Relay())
		if att, ok := id.Attestation(); ok {
			_, _ = fmt.Fprintf(w, "  - set BUZZ_ALLOW_NIP_OA_AUTH=true on the relay and restart it; the owner\n"+
				"    %s must be a relay member\n", att.Owner().Npub())
		}
		_, _ = fmt.Fprintf(w, "  - add the agent %s as a relay member\n", agent)
	case shared.IsCode(err, shared.CodeChannelNotVisible):
		_, _ = fmt.Fprintf(w, "\nThe agent cannot join %s by itself (a private channel, or a name it cannot see).\n"+
			"In Buzz Desktop open the channel, choose Members and add\n  %s\nthen run this command again.\n",
			strconv.Quote(printable(channel)), agent)
	case err != nil:
		if h := hint(err); h != "" {
			_, _ = fmt.Fprintf(w, "\n%s\n", h)
		}
	case channel == "":
		_, _ = fmt.Fprintln(w, "\nNo -channel given: run this command again with -channel <name> to join one.")
	}
}

// printGrok ends the run with the Grok Bot prompt between ``` fences, ready to
// copy. The prompt holds the agent key, so it is written by the identity store,
// the one way a secret leaves the process (as with "-out -").
func printGrok(ctx context.Context, w io.Writer, id identity.AgentIdentity, err error) error {
	_, _ = fmt.Fprintln(w, "\nGrok Bot: copy everything between the ``` lines into a chat with your Grok Bot.\n"+
		"It installs the Buzz connection on its cloud computer and from then on answers\n"+
		"mentions within 5 minutes. The prompt holds the agent key: share it with nobody else.")
	if err != nil {
		_, _ = fmt.Fprintln(w, "It can reach Buzz only once the step above is done.")
	}
	_, _ = fmt.Fprintln(w, "```")
	store, serr := envfile.New(envfile.Stdout, envfile.FormatGrok, envfile.WithStdout(w))
	if serr == nil {
		serr = store.Save(ctx, id)
	}
	_, _ = fmt.Fprintln(w, "```")
	return serr
}
