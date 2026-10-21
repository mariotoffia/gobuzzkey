package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/mariotoffia/gobuzzkey/adapters/crypto/secp256k1"
	"github.com/mariotoffia/gobuzzkey/adapters/relay/buzzhttp"
	"github.com/mariotoffia/gobuzzkey/adapters/store/envfile"
	"github.com/mariotoffia/gobuzzkey/app"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

func cmdCheck(ctx context.Context, e env) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	envPath := fs.String("env", defaultIdentityPath, "identity file (env or JSON)")
	channel := fs.String("channel", "", "channel name or ID to read")
	send := fs.String("send", "", "post this text to -channel as the agent")
	history := fs.Int("history", 0, "recent messages to read from -channel (default 3)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(e.stderr, "Usage: gobuzzkey check [-env <file>] [-channel <name|id>] [-send <text>]\n\n"+
			"Signs in to the relay as the agent, lists the channels it can see and, with\n"+
			"-channel, reads that channel and optionally posts to it.\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, e.args); err != nil {
		return err
	}
	if fs.NArg() > 0 || (*send != "" && *channel == "") {
		fs.Usage()
		return errUsage
	}

	crypto := secp256k1.New()
	store, err := envfile.New(*envPath, envfile.FormatEnv)
	if err != nil {
		return err
	}
	id, err := app.NewInspector(crypto, crypto, e.clock).Open(ctx, store)
	if err != nil {
		return err
	}
	client := buzzhttp.New(id, crypto, buzzhttp.WithClock(e.clock))

	rep, err := app.NewChecker(crypto, e.clock).Check(ctx, id, client, app.CheckRequest{
		Channel: *channel,
		Send:    *send,
		History: *history,
	})
	printCheck(e.stdout, id.Agent().Npub(), rep, rep.Channel.ID != "", *send != "")
	return err
}

// maxFieldOutput caps one relay-supplied field on screen.
const maxFieldOutput = 200

// printable makes relay-supplied text terminal-safe; any relay member can
// choose a channel name.
func printable(s string) string { return shared.Printable(s, maxFieldOutput) }

func printCheck(w io.Writer, agent string, rep app.CheckReport, readChannel, sent bool) {
	if !rep.Admitted {
		return
	}
	_, _ = fmt.Fprintf(w, "relay admitted agent %s\n\n", agent)
	_, _ = fmt.Fprintf(w, "channels visible to the agent (%d):\n", len(rep.Channels))
	for _, ch := range rep.Channels {
		_, _ = fmt.Fprintf(w, "  %-32s %s\n", printable(ch.Name), printable(ch.ID))
	}
	if !readChannel {
		return
	}
	_, _ = fmt.Fprintf(w, "\n%s (%s): read ok, %d recent message(s)\n", printable(rep.Channel.Name), printable(rep.Channel.ID), rep.Recent)
	if sent && rep.Posted.EventID != "" {
		status := "accepted"
		if !rep.Posted.Accepted {
			status = "rejected: " + rep.Posted.Message
		}
		_, _ = fmt.Fprintf(w, "posted %s (%s)\n", printable(rep.Posted.EventID), printable(status))
	}
}
