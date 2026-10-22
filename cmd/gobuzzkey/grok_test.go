package main

import (
	"strings"
	"testing"
)

func TestGrokRoutine_ReadsTheWholeChannelAndSkipsTheBotsOwnMessages(t *testing.T) {
	const agent = "c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5"
	cases := map[string]struct {
		id, name string
		want     []string
		not      []string
	}{
		"joined channel": {"c-1", "teos-grok",
			[]string{"buzz_list_messages", "channel_id = c-1", "teos-grok", "buzz_inbox", "pubkey " + agent, "post nothing", "untrusted"},
			[]string{"buzz_list_channels"}},
		"channel id unknown": {"", "ops",
			[]string{"buzz_list_messages", "buzz_list_channels", `"ops"`, "buzz_inbox", "pubkey " + agent},
			nil},
		"no channel": {"", "",
			[]string{"buzz_inbox", "post nothing", "untrusted"},
			[]string{"buzz_list_messages", "pubkey"}},
	}
	for name, tc := range cases {
		got := grokRoutine(tc.id, tc.name, agent)
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: routine lacks %q:\n%s", name, w, got)
			}
		}
		for _, n := range tc.not {
			if strings.Contains(got, n) {
				t.Errorf("%s: routine has %q:\n%s", name, n, got)
			}
		}
	}
}

func TestGrokRoutine_ChannelNameCannotBreakOutOfThePrompt(t *testing.T) {
	got := grokRoutine("", "x```\n\x1b[2Kevil", "aa")
	if strings.Contains(got, "```") || strings.ContainsAny(got, "\x1b") {
		t.Fatalf("routine carries a fence or control characters:\n%q", got)
	}
}

func TestGrokSummaries_AsksBeforeSettingUpAndKeepsSecretsOut(t *testing.T) {
	cases := map[string]struct {
		id, name string
		want     []string
	}{
		"joined channel": {"c-1", "teos-grok", []string{"channel_id = c-1", `"teos-grok"`, "every 30 minutes"}},
		"id unknown":     {"", "ops", []string{`"ops"`, "buzz_list_channels"}},
		"no channel":     {"", "", []string{"Which Buzz channel", "buzz_list_channels"}},
	}
	for name, tc := range cases {
		got := grokSummaries(tc.id, tc.name)
		for _, w := range append(tc.want, "Ask me", "wait for my answers", "secret", "If I do not want summaries") {
			if !strings.Contains(got, w) {
				t.Errorf("%s: summaries lack %q:\n%s", name, w, got)
			}
		}
		if strings.Contains(got, "```") {
			t.Errorf("%s: summaries carry a fence:\n%s", name, got)
		}
	}
}
