package main

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// Exit statuses. Scripts branch on these; README.md lists them.
const (
	exitOK        = 0
	exitFailure   = 1   // anything not classified below
	exitUsage     = 2   // bad command line
	exitInput     = 3   // a key, tag, conditions, relay URL or identity is invalid
	exitTrust     = 4   // signature invalid, self-attestation or attestation expired
	exitAdmission = 5   // the relay refused the agent (membership or NIP-98)
	exitRelay     = 6   // the relay rejected a request or is unavailable
	exitChannel   = 7   // the channel is not visible to the agent, or its name is ambiguous
	exitInterrupt = 130 // interrupted (Ctrl-C), the shell convention 128+SIGINT
)

// maxErrorOutput caps an error line; relay text inside it is capped earlier.
const maxErrorOutput = 2000

// errUsage marks a command-line error already explained to the user.
var errUsage = errors.New("usage")

func exitCode(err error) int {
	if err == nil {
		return exitOK
	}
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	if errors.Is(err, errUsage) {
		return exitUsage
	}
	if errors.Is(err, context.Canceled) {
		return exitInterrupt
	}
	code, ok := shared.CodeOf(err)
	if !ok {
		return exitFailure
	}
	switch code {
	case shared.CodeInvalidKey, shared.CodeInvalidConditions, shared.CodeInvalidTag,
		shared.CodeInvalidRelayURL, shared.CodeInvalidEvent, shared.CodeIdentityInvalid,
		shared.CodeIdentityExists, shared.CodeSecretUnavailable:
		return exitInput
	case shared.CodeSignatureInvalid, shared.CodeSelfAttestation, shared.CodeAttestationExpired:
		return exitTrust
	case shared.CodeRelayMembershipRequired, shared.CodeRelayUnauthorized:
		return exitAdmission
	case shared.CodeRelayRejected, shared.CodeRelayUnavailable:
		return exitRelay
	case shared.CodeChannelNotVisible, shared.CodeChannelAmbiguous:
		return exitChannel
	default:
		return exitFailure
	}
}

// hint returns the next step for a classified error.
func hint(err error) string {
	code, _ := shared.CodeOf(err)
	switch code {
	case shared.CodeRelayMembershipRequired:
		return "The relay did not admit the agent. Either the relay does not accept owner\n" +
			"attestations (Buzz: BUZZ_ALLOW_NIP_OA_AUTH is false, the default) or the owner\n" +
			"key is not a relay member. Ask the relay operator to enable NIP-OA admission or\n" +
			"to add the agent npub as a relay member."
	case shared.CodeRelayUnauthorized:
		return "The relay rejected the NIP-98 signature. Check that the machine clock is correct."
	case shared.CodeChannelNotVisible:
		return "Add the agent npub to the channel in Buzz (channel members, \"paste a public key\"), then run check again."
	case shared.CodeAttestationExpired:
		return "Mint a new identity with gobuzzkey new and replace the agent's BUZZ_PRIVATE_KEY and BUZZ_AUTH_TAG."
	case shared.CodeIdentityExists:
		return "gobuzzkey never overwrites an identity. Pass -out with another path."
	case shared.CodeChannelAmbiguous:
		return "Channel names are not unique in Buzz. Pass the channel ID with -channel."
	default:
		return ""
	}
}

// parseFlags parses args; any flag error is a usage error (the flag package
// has already printed it), and -h is a successful request for help.
func parseFlags(fs *flag.FlagSet, args []string) error {
	switch err := fs.Parse(args); {
	case err == nil:
		return nil
	case errors.Is(err, flag.ErrHelp):
		return flag.ErrHelp
	default:
		return errUsage
	}
}

func report(e env, err error) int {
	if err == nil || errors.Is(err, errUsage) || errors.Is(err, flag.ErrHelp) {
		return exitCode(err)
	}
	_, _ = fmt.Fprintln(e.stderr, "error:", shared.Printable(err.Error(), maxErrorOutput))
	if h := hint(err); h != "" {
		_, _ = fmt.Fprintln(e.stderr)
		_, _ = fmt.Fprintln(e.stderr, h)
	}
	return exitCode(err)
}
