package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mariotoffia/gobuzzkey/domain/clock/clocktest"
	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/testutil/fakerelay"
)

// nipoaRelay is a relay that admits agents attested by owner, with one open
// channel teos-grok (c-1) and one private channel ops (c-2).
func nipoaRelay(t *testing.T, owner key.PublicKey) *fakerelay.Relay {
	t.Helper()
	r := fakerelay.Start(fakerelay.WithClock(clocktest.At(now)), fakerelay.WithNIPOA(true), fakerelay.WithMembers(owner),
		fakerelay.WithChannels(
			fakerelay.Channel{ID: "c-1", Name: "teos-grok"},
			fakerelay.Channel{ID: "c-2", Name: "ops", Private: true},
		))
	t.Cleanup(r.Close)
	return r
}

// grokPrompt splits new's output into the fenced Grok Bot prompt and
// everything outside it.
func grokPrompt(t *testing.T, out string) (prompt, outside string) {
	t.Helper()
	before, rest, ok := strings.Cut(out, "```\n")
	prompt, after, ok2 := strings.Cut(rest, "```\n")
	if !ok || !ok2 || !strings.HasPrefix(prompt, "Add a custom MCP server") {
		t.Fatalf("no fenced Grok Bot prompt in:\n%s", out)
	}
	return prompt, before + after
}

func writes(r *fakerelay.Relay) int {
	n := 0
	for _, req := range r.Requests() {
		if req.Path == "/events" {
			n++
		}
	}
	return n
}

func TestNew_OneCommandMintsJoinsAnOpenChannelAndChecks(t *testing.T) {
	ownerSK, ownerPub := owner(t)
	r := nipoaRelay(t, ownerPub)
	dir := t.TempDir()
	path := filepath.Join(dir, "buzz-agent.env")

	res := invoke(t, map[string]string{envOwnerSecret: ownerSK.Nsec()}, "new", "-relay", r.URL(), "-out", path, "-channel", "teos-grok")
	if res.code != exitOK {
		t.Fatalf("new: %d\n%s%s", res.code, res.stdout, res.stderr)
	}
	for _, want := range []string{"ok    identity  created " + path, "ok    relay", "ok    channel   joined teos-grok", "ok    check     read 0"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, res.stdout)
		}
	}
	prompt, outside := grokPrompt(t, res.stdout)
	if strings.Contains(outside+res.stderr, "nsec1") {
		t.Fatalf("a secret key was printed outside the Grok Bot prompt:\n%s", res.stdout)
	}
	agent := agentOf(t, path)
	if !slices.Contains(r.Members("c-1"), agent) {
		t.Fatal("agent did not join the open channel")
	}

	env, _ := os.ReadFile(path)
	wantExpiry := time.Unix(now, 0).UTC().AddDate(3, 0, 0).Format("2006-01-02T15:04:05Z")
	if !strings.Contains(string(env), "# expires "+wantExpiry) {
		t.Fatalf("default expiry is not 3 years:\n%s", env)
	}
	if !strings.Contains(prompt, "buzz_list_messages with channel_id = c-1") || !strings.Contains(prompt, "pubkey "+agent.Hex()) ||
		!strings.Contains(prompt, "status summaries") {
		t.Fatalf("Grok Bot routine does not read the joined channel:\n%s", prompt)
	}
	for _, line := range strings.Split(string(env), "\n") {
		if v, ok := strings.CutPrefix(line, "BUZZ_PRIVATE_KEY="); ok && !strings.Contains(prompt, "BUZZ_PRIVATE_KEY = "+v+"\n") {
			t.Fatalf("Grok Bot prompt does not carry the identity's key:\n%s", prompt)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("new wrote more than the identity file: %v", entries)
	}
}

func TestNew_RunningAgainKeepsTheIdentityAndPublishesNothing(t *testing.T) {
	ownerSK, ownerPub := owner(t)
	r := nipoaRelay(t, ownerPub)
	path := filepath.Join(t.TempDir(), "buzz-agent.env")
	if res := invoke(t, map[string]string{envOwnerSecret: ownerSK.Nsec()}, "new", "-relay", r.URL(), "-out", path, "-channel", "teos-grok"); res.code != exitOK {
		t.Fatalf("first run: %d %s", res.code, res.stderr)
	}
	before, _ := os.ReadFile(path)
	joins := writes(r)

	// No owner secret this time: a second run must not need it.
	res := invoke(t, nil, "new", "-relay", "ws"+strings.TrimPrefix(r.URL(), "http"), "-out", path, "-channel", "teos-grok")
	if res.code != exitOK {
		t.Fatalf("second run: %d\n%s%s", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "ok    identity  keeping "+path) || !strings.Contains(res.stdout, "already a member of teos-grok") {
		t.Fatalf("second run output:\n%s", res.stdout)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("identity file changed")
	}
	if writes(r) != joins {
		t.Fatalf("second run published %d event(s)", writes(r)-joins)
	}
}

func TestNew_PrivateChannelExplainsHowToAddTheAgent(t *testing.T) {
	ownerSK, ownerPub := owner(t)
	r := nipoaRelay(t, ownerPub)
	path := filepath.Join(t.TempDir(), "buzz-agent.env")
	res := invoke(t, map[string]string{envOwnerSecret: ownerSK.Nsec()}, "new", "-relay", r.URL(), "-out", path, "-channel", "ops")
	if res.code != exitChannel {
		t.Fatalf("new: %d\n%s%s", res.code, res.stdout, res.stderr)
	}
	agent := agentOf(t, path)
	grokPrompt(t, res.stdout)
	for _, want := range []string{"FAIL  channel", "Buzz Desktop", agent.Npub()} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, res.stdout)
		}
	}
	if res.stderr != "" || writes(r) != 0 {
		t.Fatalf("stderr %q, %d write(s)", res.stderr, writes(r))
	}
}

func TestNew_RelayWithoutNIPOAPrintsWhatTheAdminMustDo(t *testing.T) {
	ownerSK, ownerPub := owner(t)
	r := fakerelay.Start(fakerelay.WithClock(clocktest.At(now)), fakerelay.WithMembers(ownerPub),
		fakerelay.WithChannels(fakerelay.Channel{ID: "c-1", Name: "teos-grok"}))
	defer r.Close()
	path := filepath.Join(t.TempDir(), "buzz-agent.env")
	res := invoke(t, map[string]string{envOwnerSecret: ownerSK.Nsec()}, "new", "-relay", r.URL(), "-out", path, "-channel", "teos-grok")
	if res.code != exitAdmission {
		t.Fatalf("new: %d\n%s%s", res.code, res.stdout, res.stderr)
	}
	grokPrompt(t, res.stdout)
	for _, want := range []string{"FAIL  relay", "BUZZ_ALLOW_NIP_OA_AUTH=true", ownerPub.Npub(), agentOf(t, path).Npub()} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, res.stdout)
		}
	}
	if strings.Contains(res.stdout, "channel   ") {
		t.Fatalf("channel step ran although the relay refused the agent:\n%s", res.stdout)
	}
}

func TestNew_IdentityForAnotherRelayIsKeptAndExplained(t *testing.T) {
	ownerSK, ownerPub := owner(t)
	r := nipoaRelay(t, ownerPub)
	path := filepath.Join(t.TempDir(), "buzz-agent.env")
	if res := invoke(t, map[string]string{envOwnerSecret: ownerSK.Nsec()}, "new", "-relay", r.URL(), "-out", path); res.code != exitOK {
		t.Fatalf("first run: %s", res.stderr)
	}
	before, _ := os.ReadFile(path)
	res := invoke(t, map[string]string{envOwnerSecret: ownerSK.Nsec()}, "new", "-relay", "wss://other.example", "-out", path)
	if res.code != exitInput || !strings.Contains(res.stderr, "IDENTITY_EXISTS") || !strings.Contains(res.stderr, "gobuzzkey remove") {
		t.Fatalf("new for another relay: %d\n%s", res.code, res.stderr)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("identity file changed")
	}
}

func TestRemove_LeavesTheChannelAndDeletesTheIdentity(t *testing.T) {
	ownerSK, ownerPub := owner(t)
	r := nipoaRelay(t, ownerPub)
	path := filepath.Join(t.TempDir(), "buzz-agent.env")
	if res := invoke(t, map[string]string{envOwnerSecret: ownerSK.Nsec()}, "new", "-relay", r.URL(), "-out", path, "-channel", "teos-grok"); res.code != exitOK {
		t.Fatalf("new: %s", res.stderr)
	}
	agent := agentOf(t, path)

	res := invoke(t, nil, "remove", "-env", path, "-channel", "teos-grok")
	if res.code != exitOK || !strings.Contains(res.stdout, "left teos-grok") || !strings.Contains(res.stdout, "Grok Bot") {
		t.Fatalf("remove: %d\n%s%s", res.code, res.stdout, res.stderr)
	}
	if slices.Contains(r.Members("c-1"), agent) {
		t.Fatal("agent is still a channel member")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s still exists: %v", path, err)
	}

	res = invoke(t, nil, "remove", "-env", path, "-channel", "teos-grok")
	if res.code != exitOK || !strings.Contains(res.stdout, "does not exist") {
		t.Fatalf("second remove: %d\n%s%s", res.code, res.stdout, res.stderr)
	}
}

func TestRemove_RelayFailureKeepsTheIdentity(t *testing.T) {
	ownerSK, ownerPub := owner(t)
	r := nipoaRelay(t, ownerPub)
	path := filepath.Join(t.TempDir(), "buzz-agent.env")
	if res := invoke(t, map[string]string{envOwnerSecret: ownerSK.Nsec()}, "new", "-relay", r.URL(), "-out", path); res.code != exitOK {
		t.Fatalf("new: %s", res.stderr)
	}
	down := fakerelay.Start(fakerelay.WithStatus(503))
	defer down.Close()
	b, _ := os.ReadFile(path)
	_ = os.WriteFile(path, []byte(strings.Replace(string(b), r.URL(), down.URL(), 1)), 0o600)

	res := invoke(t, nil, "remove", "-env", path, "-channel", "teos-grok")
	if res.code != exitRelay || !strings.Contains(res.stdout, "FAIL  channel") || !strings.Contains(res.stdout, "kept") {
		t.Fatalf("remove: %d\n%s%s", res.code, res.stdout, res.stderr)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("identity deleted although the agent could not leave: %v", err)
	}
}

func TestNew_MissingOwnerKeySaysWhereToFindIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "buzz-agent.env")
	res := invoke(t, nil, "new", "-relay", "wss://relay.example", "-out", path)
	if res.code != exitInput || !strings.Contains(res.stderr, "SECRET_UNAVAILABLE") ||
		!strings.Contains(res.stderr, "Buzz Desktop: Settings, Profile, Private key") {
		t.Fatalf("new without the owner key: %d\n%s", res.code, res.stderr)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("identity written without an owner key: %v", err)
	}
}
