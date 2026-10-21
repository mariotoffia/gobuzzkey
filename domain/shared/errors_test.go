package shared_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

func TestError_RendersCodeMessageAndCause(t *testing.T) {
	err := shared.Wrap(shared.CodeIdentityInvalid, "BUZZ_AUTH_TAG", errors.New("boom"))
	if err.Error() != "IDENTITY_INVALID: BUZZ_AUTH_TAG: boom" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if shared.New(shared.CodeInvalidKey, "").Error() != "INVALID_KEY" {
		t.Fatal("empty message rendered")
	}
}

func TestIsCode_FindsCodesThroughTheChain(t *testing.T) {
	inner := shared.New(shared.CodeInvalidKey, "bad")
	outer := shared.Wrap(shared.CodeIdentityInvalid, "identity", inner)
	wrapped := fmt.Errorf("context: %w", outer)

	if code, _ := shared.CodeOf(wrapped); code != shared.CodeIdentityInvalid {
		t.Fatalf("CodeOf = %s", code)
	}
	if !shared.IsCode(wrapped, shared.CodeIdentityInvalid) || !shared.IsCode(wrapped, shared.CodeInvalidKey) {
		t.Fatal("codes in the chain not found")
	}
	if shared.IsCode(wrapped, shared.CodeRelayRejected) {
		t.Fatal("absent code found")
	}
	if shared.IsCode(errors.New("plain"), shared.CodeInvalidKey) {
		t.Fatal("plain error matched")
	}
	if _, ok := shared.CodeOf(nil); ok {
		t.Fatal("nil has a code")
	}
}

func TestPrintable_EscapesControlsAndTruncates(t *testing.T) {
	cases := map[string]string{
		"plain text é🙂":            "plain text é🙂",
		"osc52 \x1b]52;c;aGk=\x07": `osc52 \u001b]52;c;aGk=\u0007`,
		"line\nbreak\rreturn":      `line\u000abreak\u000dreturn`,
		"del \x7f c1 \u009b":       `del \u007f c1 \u009b`,
		"bad utf8 \xff":            `bad utf8 \xff`,
		"real replacement � ok":    "real replacement � ok",
	}
	for in, want := range cases {
		if got := shared.Printable(in, 100); got != want {
			t.Errorf("Printable(%q) = %q, want %q", in, got, want)
		}
	}
	if got := shared.Printable("abcdef", 3); got != "abc…" {
		t.Errorf("truncation = %q", got)
	}
}
