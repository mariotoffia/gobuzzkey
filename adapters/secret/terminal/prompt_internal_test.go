package terminal

import (
	"strings"
	"testing"
)

func TestPromptText_SaysWhereToFindTheKey(t *testing.T) {
	if !strings.Contains(promptText, "Buzz Desktop: Settings, Profile, Private key") || !strings.HasSuffix(promptText, ": ") {
		t.Fatalf("prompt = %q", promptText)
	}
}
