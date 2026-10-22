package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mariotoffia/gobuzzkey/adapters/crypto/secp256k1"
	"github.com/mariotoffia/gobuzzkey/domain/clock/clocktest"
	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
	"github.com/mariotoffia/gobuzzkey/testutil/fakerelay"
)

const now = 1800000000

type result struct {
	code           int
	stdout, stderr string
}

func invoke(t *testing.T, vars map[string]string, args ...string) result {
	t.Helper()
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = devnull.Close() }()
	var out, errOut bytes.Buffer
	code := run(context.Background(), env{
		args:   args,
		getenv: func(k string) string { return vars[k] },
		stdin:  devnull,
		stdout: &out,
		stderr: &errOut,
		clock:  clocktest.At(now),
	})
	return result{code: code, stdout: out.String(), stderr: errOut.String()}
}

func owner(t *testing.T) (key.SecretKey, key.PublicKey) {
	t.Helper()
	c := secp256k1.New()
	sk, err := c.Generate()
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := c.PublicKey(sk)
	return sk, pub
}

func agentOf(t *testing.T, path string) key.PublicKey {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if npub, ok := strings.CutPrefix(line, "# agent "); ok {
			pub, err := key.ParsePublicKey(npub)
			if err != nil {
				t.Fatal(err)
			}
			return pub
		}
	}
	t.Fatal("no agent comment")
	return key.PublicKey{}
}

func TestNewVerifyCheck_EndToEndAgainstARelayThatAcceptsAttestations(t *testing.T) {
	ownerSK, ownerPub := owner(t)
	vars := map[string]string{envOwnerSecret: ownerSK.Nsec()}
	path := filepath.Join(t.TempDir(), "agent.env")

	r := fakerelay.Start(fakerelay.WithClock(clocktest.At(now)), fakerelay.WithNIPOA(true), fakerelay.WithMembers(ownerPub))
	defer r.Close()

	res := invoke(t, vars, "new", "-relay", r.URL(), "-out", path, "-expires", "90d")
	if res.code != exitOK {
		t.Fatalf("new: %d %s", res.code, res.stderr)
	}
	if _, outside := grokPrompt(t, res.stdout); !strings.Contains(outside, "owner    "+ownerPub.Npub()) || strings.Contains(outside+res.stderr, "nsec1") {
		t.Fatalf("new output:\n%s", res.stdout)
	}

	res = invoke(t, nil, "verify", "-env", path)
	if res.code != exitOK || !strings.Contains(res.stdout, "status      admissible now") {
		t.Fatalf("verify: %d\n%s%s", res.code, res.stdout, res.stderr)
	}
	wantExpiry := time.Unix(now, 0).UTC().Add(90 * 24 * time.Hour).Format(time.RFC3339)
	if !strings.Contains(res.stdout, "expires     "+wantExpiry) {
		t.Fatalf("verify expiry:\n%s", res.stdout)
	}

	res = invoke(t, nil, "check", "-env", path, "-channel", "teos-grok")
	if res.code != exitChannel || !strings.Contains(res.stdout, "relay admitted agent") {
		t.Fatalf("check before the agent joins: %d\n%s%s", res.code, res.stdout, res.stderr)
	}

	r2 := fakerelay.Start(fakerelay.WithClock(clocktest.At(now)), fakerelay.WithNIPOA(true), fakerelay.WithMembers(ownerPub),
		fakerelay.WithChannels(fakerelay.Channel{ID: "c-1", Name: "teos-grok", Members: []key.PublicKey{agentOf(t, path)}}))
	defer r2.Close()
	path2 := filepath.Join(t.TempDir(), "agent2.env")
	b, _ := os.ReadFile(path)
	_ = os.WriteFile(path2, bytes.Replace(b, []byte(r.URL()), []byte(r2.URL()), 1), 0o600)

	res = invoke(t, nil, "check", "-env", path2, "-channel", "teos-grok", "-send", "hello")
	if res.code != exitOK || !strings.Contains(res.stdout, "teos-grok (c-1): read ok") || !strings.Contains(res.stdout, "(accepted)") {
		t.Fatalf("check after the agent joins: %d\n%s%s", res.code, res.stdout, res.stderr)
	}
	if msgs := r2.Messages(); len(msgs) != 1 || msgs[0].Content != "hello" {
		t.Fatalf("relay messages = %+v", msgs)
	}
}

func TestCheck_RelayWithoutNIPOAExitsWithAdmissionHint(t *testing.T) {
	ownerSK, ownerPub := owner(t)
	r := fakerelay.Start(fakerelay.WithClock(clocktest.At(now)), fakerelay.WithMembers(ownerPub))
	defer r.Close()
	path := filepath.Join(t.TempDir(), "agent.env")
	if res := invoke(t, map[string]string{envOwnerSecret: ownerSK.Nsec()}, "new", "-relay", r.URL(), "-out", path); res.code != exitAdmission {
		t.Fatalf("new: %d %s", res.code, res.stderr)
	}
	res := invoke(t, nil, "check", "-env", path)
	if res.code != exitAdmission || !strings.Contains(res.stderr, "BUZZ_ALLOW_NIP_OA_AUTH") {
		t.Fatalf("check: %d\n%s", res.code, res.stderr)
	}
}

func TestNew_NeverOverwritesAnUnreadableFileNorAsksForTheSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.env")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	res := invoke(t, nil, "new", "-relay", "wss://relay.example", "-out", path)
	if res.code != exitInput || !strings.Contains(res.stderr, "IDENTITY_INVALID") || strings.Contains(res.stderr, "Owner secret") {
		t.Fatalf("new: %d %s", res.code, res.stderr)
	}
	if b, _ := os.ReadFile(path); string(b) != "keep" {
		t.Fatalf("file overwritten: %q", b)
	}
}

func TestNew_DashWritesJSONToStdoutAndStatusToStderr(t *testing.T) {
	ownerSK, ownerPub := owner(t)
	r := nipoaRelay(t, ownerPub)
	res := invoke(t, map[string]string{envOwnerSecret: ownerSK.Nsec()}, "new", "-relay", r.URL(), "-out", "-", "-format", "json")
	if res.code != exitOK {
		t.Fatalf("new: %s", res.stderr)
	}
	if !strings.HasPrefix(strings.TrimSpace(res.stdout), "{") || !strings.Contains(res.stdout, `"BUZZ_AUTH_TAG"`) {
		t.Fatalf("stdout = %s", res.stdout)
	}
	if !strings.Contains(res.stderr, "identity  written to stdout") || strings.Contains(res.stderr, "grok") {
		t.Fatalf("stderr = %s", res.stderr)
	}
}

func TestVerify_TagModeAndSignatureFailure(t *testing.T) {
	ownerSK, ownerPub := owner(t)
	r := nipoaRelay(t, ownerPub)
	var out bytes.Buffer
	res := invoke(t, map[string]string{envOwnerSecret: ownerSK.Nsec()}, "new", "-relay", r.URL(), "-out", "-")
	out.WriteString(res.stdout)
	var tag, agent string
	for _, line := range strings.Split(out.String(), "\n") {
		if v, ok := strings.CutPrefix(line, "BUZZ_AUTH_TAG='"); ok {
			tag = strings.TrimSuffix(v, "'")
		}
		if v, ok := strings.CutPrefix(line, "# agent "); ok {
			agent = v
		}
	}
	if res := invoke(t, nil, "verify", "-tag", tag, "-agent", agent); res.code != exitOK {
		t.Fatalf("verify tag: %d %s", res.code, res.stderr)
	}
	_, stranger := owner(t)
	if res := invoke(t, nil, "verify", "-tag", tag, "-agent", stranger.Npub()); res.code != exitTrust {
		t.Fatalf("verify for another agent: %d %s", res.code, res.stderr)
	}
}

func TestRun_UsageErrors(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"bogus"},
		{"new"},
		{"new", "-relay", "wss://r.example", "extra"},
		{"verify", "-tag", "x"},
		{"check", "-send", "hi"},
		{"new", "-nope"},
		{"new", "-relay", "wss://r.example", "-format", "yaml"},
		{"new", "-relay", "wss://r.example", "-format", "grok"},
		{"remove", "extra"},
		{"remove", "-nope"},
	} {
		if res := invoke(t, nil, args...); res.code != exitUsage {
			t.Errorf("%v: exit %d", args, res.code)
		}
	}
	if res := invoke(t, nil, "new", "-h"); res.code != exitOK || !strings.Contains(res.stderr, "-relay") {
		t.Fatalf("new -h: %+v", res)
	}
	if res := invoke(t, nil, "version"); res.code != exitOK || !strings.HasPrefix(res.stdout, "gobuzzkey ") {
		t.Fatalf("version: %+v", res)
	}
}

func TestExitCode_MapsEveryErrorCode(t *testing.T) {
	cases := map[shared.Code]int{
		shared.CodeInvalidKey:              exitInput,
		shared.CodeInvalidConditions:       exitInput,
		shared.CodeInvalidTag:              exitInput,
		shared.CodeInvalidRelayURL:         exitInput,
		shared.CodeInvalidEvent:            exitInput,
		shared.CodeIdentityInvalid:         exitInput,
		shared.CodeIdentityExists:          exitInput,
		shared.CodeSecretUnavailable:       exitInput,
		shared.CodeSelfAttestation:         exitTrust,
		shared.CodeSignatureInvalid:        exitTrust,
		shared.CodeAttestationExpired:      exitTrust,
		shared.CodeRelayMembershipRequired: exitAdmission,
		shared.CodeRelayUnauthorized:       exitAdmission,
		shared.CodeRelayRejected:           exitRelay,
		shared.CodeRelayUnavailable:        exitRelay,
		shared.CodeChannelNotVisible:       exitChannel,
		shared.CodeChannelAmbiguous:        exitChannel,
	}
	for code, want := range cases {
		if got := exitCode(shared.New(code, "")); got != want {
			t.Errorf("%s -> %d, want %d", code, got, want)
		}
	}
	interrupted := shared.Wrap(shared.CodeSecretUnavailable, "interrupted", context.Canceled)
	if got := exitCode(interrupted); got != exitInterrupt {
		t.Errorf("interrupted -> %d, want %d", got, exitInterrupt)
	}
}

func TestCheck_RelayTextCannotReachTheTerminalRaw(t *testing.T) {
	ownerSK, ownerPub := owner(t)
	path := filepath.Join(t.TempDir(), "agent.env")
	r := fakerelay.Start(fakerelay.WithClock(clocktest.At(now)), fakerelay.WithNIPOA(true), fakerelay.WithMembers(ownerPub))
	defer r.Close()
	if res := invoke(t, map[string]string{envOwnerSecret: ownerSK.Nsec()}, "new", "-relay", r.URL(), "-out", path); res.code != exitOK {
		t.Fatalf("new: %s", res.stderr)
	}
	evil := "x\x1b]52;c;aGk=\x07\x1b[2Ky"
	r2 := fakerelay.Start(fakerelay.WithClock(clocktest.At(now)), fakerelay.WithNIPOA(true), fakerelay.WithMembers(ownerPub),
		fakerelay.WithChannels(fakerelay.Channel{ID: "c-1", Name: evil, Members: []key.PublicKey{agentOf(t, path)}}))
	defer r2.Close()
	b, _ := os.ReadFile(path)
	_ = os.WriteFile(path, bytes.Replace(b, []byte(r.URL()), []byte(r2.URL()), 1), 0o600)

	res := invoke(t, nil, "check", "-env", path, "-channel", "c-1")
	if res.code != exitOK {
		t.Fatalf("check: %d %s", res.code, res.stderr)
	}
	if strings.ContainsAny(res.stdout+res.stderr, "\x1b\x07") || !strings.Contains(res.stdout, `x\u001b]52`) {
		t.Fatalf("control characters reached the terminal: %q", res.stdout)
	}
}

func TestParseExpiry(t *testing.T) {
	base := time.Unix(now, 0).UTC()
	cases := map[string]time.Time{
		"":                     {},
		"2027-10-31":           time.Date(2027, 10, 31, 0, 0, 0, 0, time.UTC),
		"2027-10-31T12:00:00Z": time.Date(2027, 10, 31, 12, 0, 0, 0, time.UTC),
		"90d":                  base.Add(90 * 24 * time.Hour),
		"3y":                   base.AddDate(3, 0, 0),
		"50y":                  base.AddDate(50, 0, 0),
		"36h":                  base.Add(36 * time.Hour),
		"2106-02-07T06:28:15Z": time.Unix(4294967295, 0).UTC(),
	}
	for in, want := range cases {
		got, err := parseExpiry(in, base)
		if err != nil || !got.Equal(want) {
			t.Errorf("%q -> %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{
		"tomorrow", "0d", "-5h", "31/10/2027", "0y", "-1y", "1.5y",
		"101y",                 // beyond the year cap
		"90y",                  // after 2106-02-07, as a number of years
		"213594d",              // would overflow time.Duration
		"36501d",               // beyond the day cap
		"30000d",               // after 2106-02-07, the last created_at NIP-OA can express
		"2200-01-01",           // same, as a date
		"2106-02-07T06:28:16Z", // one second past the limit
	} {
		if _, err := parseExpiry(in, base); !shared.IsCode(err, shared.CodeInvalidConditions) {
			t.Errorf("%q accepted: %v", in, err)
		}
	}
}
