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
	for _, want := range []string{"ok    identity  created " + path, "ok    relay", "ok    channel   joined teos-grok", "ok    check     read 0", "buzz-agent.grok.txt"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, res.stdout)
		}
	}
	if strings.Contains(res.stdout+res.stderr, "nsec1") {
		t.Fatalf("a secret key reached the terminal:\n%s", res.stdout)
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
	grok := filepath.Join(dir, "buzz-agent.grok.txt")
	msg, err := os.ReadFile(grok)
	if err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(grok); info.Mode().Perm() != 0o600 {
		t.Fatalf("grok message mode = %v", info.Mode().Perm())
	}
	for _, line := range strings.Split(string(env), "\n") {
		if v, ok := strings.CutPrefix(line, "BUZZ_PRIVATE_KEY="); ok && !strings.Contains(string(msg), "BUZZ_PRIVATE_KEY = "+v+"\n") {
			t.Fatalf("grok message does not carry the identity's key:\n%s", msg)
		}
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
	for _, want := range []string{"FAIL  channel", "Buzz Desktop", agent.Npub(), "buzz-agent.grok.txt"} {
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
	for _, want := range []string{"FAIL  relay", "BUZZ_ALLOW_NIP_OA_AUTH=true", ownerPub.Npub(), agentOf(t, path).Npub(), "buzz-agent.grok.txt"} {
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

func TestRemove_LeavesTheChannelAndDeletesBothFiles(t *testing.T) {
	ownerSK, ownerPub := owner(t)
	r := nipoaRelay(t, ownerPub)
	dir := t.TempDir()
	path := filepath.Join(dir, "buzz-agent.env")
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
	for _, p := range []string{path, filepath.Join(dir, "buzz-agent.grok.txt")} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s still exists: %v", p, err)
		}
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
