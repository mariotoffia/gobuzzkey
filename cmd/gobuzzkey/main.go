// Command gobuzzkey mints owner-attested (NIP-OA) agent identities for Buzz
// relays and checks that a relay admits them. It is the composition root:
// the only place that reads the environment and wires adapters to use cases.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"

	"github.com/mariotoffia/gobuzzkey/domain/clock"
)

const usage = `gobuzzkey — owner-attested agent identities for Buzz relays

Usage:
  gobuzzkey new     -relay <url> [-channel <name|id>] [-out buzz-agent.env|-] [-format env|json] [-expires 3y]
  gobuzzkey verify  [-env buzz-agent.env] | -tag <json> -agent <npub|hex>
  gobuzzkey check   [-env buzz-agent.env] [-channel <name|id>] [-send <text>] [-history <n>]
  gobuzzkey remove  [-env buzz-agent.env] [-channel <name|id>]
  gobuzzkey version

Run "gobuzzkey <command> -h" for the flags of one command.
`

// env is the composition root's view of the process environment.
type env struct {
	args   []string
	getenv func(string) string
	stdin  *os.File
	stdout io.Writer
	stderr io.Writer
	clock  clock.Clock
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, env{
		args:   os.Args[1:],
		getenv: os.Getenv,
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
		clock:  clock.Real{},
	})
	stop()
	os.Exit(code)
}

func run(ctx context.Context, e env) int {
	if len(e.args) == 0 {
		_, _ = io.WriteString(e.stderr, usage)
		return exitUsage
	}
	cmd, rest := e.args[0], e.args[1:]
	e.args = rest
	switch cmd {
	case "new":
		return report(e, cmdNew(ctx, e))
	case "verify":
		return report(e, cmdVerify(ctx, e))
	case "check":
		return report(e, cmdCheck(ctx, e))
	case "remove":
		return report(e, cmdRemove(ctx, e))
	case "version", "-version", "--version":
		_, _ = fmt.Fprintln(e.stdout, "gobuzzkey", version())
		return exitOK
	case "help", "-h", "--help":
		_, _ = io.WriteString(e.stdout, usage)
		return exitOK
	default:
		_, _ = fmt.Fprintf(e.stderr, "unknown command %q\n\n%s", cmd, usage)
		return exitUsage
	}
}

func version() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "(devel)"
}
