package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/mariotoffia/gobuzzkey/adapters/crypto/secp256k1"
	"github.com/mariotoffia/gobuzzkey/adapters/relay/buzzhttp"
	"github.com/mariotoffia/gobuzzkey/adapters/store/envfile"
	"github.com/mariotoffia/gobuzzkey/app"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

func cmdRemove(ctx context.Context, e env) error {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	envPath := fs.String("env", defaultIdentityPath, "identity file (env or JSON)")
	channel := fs.String("channel", "", "channel name or ID the agent leaves first")
	fs.Usage = func() {
		_, _ = fmt.Fprint(e.stderr, "Usage: gobuzzkey remove [-env <file>] [-channel <name|id>]\n\n"+
			"Undoes gobuzzkey new and is safe to run again: the agent leaves -channel, then\n"+
			"the identity file is deleted. The attestation cannot be withdrawn, but without\n"+
			"the agent key nobody can use it.\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, e.args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return errUsage
	}

	w := e.stdout
	store, err := envfile.New(*envPath, envfile.FormatEnv)
	if err != nil {
		return err
	}
	// Writable reports an existing file as IDENTITY_EXISTS; without one there is
	// no agent key left to leave a channel with.
	exists := shared.IsCode(store.Writable(ctx), shared.CodeIdentityExists)
	if *channel != "" && exists {
		if err := leave(ctx, e, store, *channel); err != nil {
			_, _ = fmt.Fprintf(w, "\n%s is kept so you can run this again. Run without -channel to delete it anyway.\n", *envPath)
			return explained{err}
		}
	}
	removed, err := store.Remove(ctx)
	switch {
	case err != nil:
		step(w, err, "identity", "")
		return explained{err}
	case removed:
		step(w, nil, "identity", "deleted "+*envPath)
	default:
		step(w, nil, "identity", *envPath+" does not exist")
	}
	_, _ = fmt.Fprintln(w, "\nAlso remove the \"buzz\" MCP server from your Grok Bot (Settings, Plugins, Yours):\n"+
		"it still holds a copy of the agent key.")
	return nil
}

// leave takes the agent out of channel. A channel the agent cannot see is one
// it is not a member of.
func leave(ctx context.Context, e env, store *envfile.Store, channel string) error {
	w := e.stdout
	crypto := secp256k1.New()
	id, err := app.NewInspector(crypto, crypto, e.clock).Open(ctx, store)
	if err != nil {
		step(w, err, "channel", "")
		return err
	}
	client := buzzhttp.New(id, crypto, buzzhttp.WithClock(e.clock))
	m, err := app.NewChecker(crypto, e.clock).Leave(ctx, id, client, channel)
	switch {
	case shared.IsCode(err, shared.CodeChannelNotVisible):
		step(w, nil, "channel", "the agent is not in "+printable(channel))
	case err != nil:
		step(w, err, "channel", "")
		return err
	case m.Changed:
		step(w, nil, "channel", "left "+printable(m.Channel.Name)+" ("+printable(m.Channel.ID)+")")
	default:
		step(w, nil, "channel", "the agent is not in "+printable(m.Channel.Name))
	}
	return nil
}
