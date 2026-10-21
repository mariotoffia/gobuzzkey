// Package shared is the shared kernel of gobuzzkey: the structured error type
// every layer returns and the error codes callers branch on.
package shared

import (
	"errors"
	"strings"
)

// Code identifies a class of failure. Callers branch on the Code with IsCode,
// never on the message text.
type Code string

// Error codes. The command maps each to an exit status and a hint
// (see UBIQUITOUS.md, "Error codes").
const (
	// CodeInvalidKey: a secret or public key failed to parse or is not a valid
	// secp256k1 key.
	CodeInvalidKey Code = "INVALID_KEY"
	// CodeInvalidConditions: a NIP-OA conditions string breaks the grammar.
	CodeInvalidConditions Code = "INVALID_CONDITIONS"
	// CodeInvalidTag: an auth tag is not a well-formed 4-element NIP-OA tag.
	CodeInvalidTag Code = "INVALID_TAG"
	// CodeSelfAttestation: the owner and the agent are the same key.
	CodeSelfAttestation Code = "SELF_ATTESTATION"
	// CodeSignatureInvalid: the owner signature does not verify for the agent.
	CodeSignatureInvalid Code = "SIGNATURE_INVALID"
	// CodeAttestationExpired: the attestation's created_at< bound has passed.
	CodeAttestationExpired Code = "ATTESTATION_EXPIRED"
	// CodeInvalidRelayURL: the relay URL is not ws, wss, http or https.
	CodeInvalidRelayURL Code = "INVALID_RELAY_URL"
	// CodeInvalidEvent: a Nostr event or filter could not be built or decoded.
	CodeInvalidEvent Code = "INVALID_EVENT"
	// CodeRelayMembershipRequired: the relay refused the agent because neither
	// the agent nor an attesting owner is a relay member.
	CodeRelayMembershipRequired Code = "RELAY_MEMBERSHIP_REQUIRED"
	// CodeRelayUnauthorized: the relay rejected the NIP-98 signature.
	CodeRelayUnauthorized Code = "RELAY_UNAUTHORIZED"
	// CodeRelayRejected: the relay answered with any other client error.
	CodeRelayRejected Code = "RELAY_REJECTED"
	// CodeRelayUnavailable: the relay could not be reached or failed itself.
	CodeRelayUnavailable Code = "RELAY_UNAVAILABLE"
	// CodeChannelNotVisible: the named channel is not visible to the agent.
	CodeChannelNotVisible Code = "CHANNEL_NOT_VISIBLE"
	// CodeChannelAmbiguous: more than one visible channel has the given name.
	CodeChannelAmbiguous Code = "CHANNEL_AMBIGUOUS"
	// CodeIdentityExists: the identity destination already exists.
	CodeIdentityExists Code = "IDENTITY_EXISTS"
	// CodeIdentityInvalid: a stored identity is missing fields or malformed.
	CodeIdentityInvalid Code = "IDENTITY_INVALID"
	// CodeSecretUnavailable: the owner secret could not be read.
	CodeSecretUnavailable Code = "SECRET_UNAVAILABLE"
)

// Error is the structured error returned across every layer.
type Error struct {
	Code    Code
	Message string
	Cause   error
}

// New returns an Error without a cause.
func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Wrap returns an Error that wraps cause.
func Wrap(code Code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

// Error renders "CODE: message: cause".
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(string(e.Code))
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if e.Cause != nil {
		b.WriteString(": ")
		b.WriteString(e.Cause.Error())
	}
	return b.String()
}

// Unwrap returns the cause.
func (e *Error) Unwrap() error { return e.Cause }

// CodeOf returns the Code of the outermost *Error in err's chain.
func CodeOf(err error) (Code, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Code, true
	}
	return "", false
}

// IsCode reports whether any *Error in err's chain carries code.
func IsCode(err error, code Code) bool {
	for err != nil {
		var e *Error
		if !errors.As(err, &e) {
			return false
		}
		if e.Code == code {
			return true
		}
		err = e.Cause
	}
	return false
}
