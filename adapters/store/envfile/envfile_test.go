package envfile_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mariotoffia/gobuzzkey/adapters/store/envfile"
	"github.com/mariotoffia/gobuzzkey/domain/attestation"
	"github.com/mariotoffia/gobuzzkey/domain/identity"
	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/relay"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// NIP-OA spec keys: owner secret 1, agent secret 2.
const (
	agentSK  = "0000000000000000000000000000000000000000000000000000000000000002"
	agentHex = "c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5"
	specTag  = `["auth","79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","kind=1&created_at<1713957000","8b7df2575caf0a108374f8471722b233c53f9ff827a8b0f91861966c3b9dd5cb2e189eae9f49d72187674c2f5bd244145e10ff86c9f257ffe65a1ee5f108b369"]`
)

func specIdentity(t *testing.T) identity.AgentIdentity {
	t.Helper()
	u, _ := relay.ParseURL("wss://relay.example")
	sk, _ := key.ParseSecretKey(agentSK)
	agent, _ := key.ParsePublicKeyHex(agentHex)
	tag, err := attestation.ParseAuthTag(specTag)
	if err != nil {
		t.Fatal(err)
	}
	id, err := identity.NewAttested(u, sk, agent, tag)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSaveLoad_RoundTripsEnvAndJSON(t *testing.T) {
	id := specIdentity(t)
	for _, format := range []envfile.Format{envfile.FormatEnv, envfile.FormatJSON} {
		t.Run(string(format), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "agent")
			s, err := envfile.New(path, format)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Save(context.Background(), id); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("mode = %v", info.Mode().Perm())
			}
			rec, err := s.Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if rec.RelayURL != "https://relay.example" || rec.PrivateKey != id.Secret().Nsec() || rec.AuthTag != specTag {
				t.Fatalf("record = %+v", rec)
			}
		})
	}
}

func TestSave_NeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.env")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := envfile.New(path, envfile.FormatEnv)
	if err := s.Writable(context.Background()); !shared.IsCode(err, shared.CodeIdentityExists) {
		t.Fatalf("Writable: want IDENTITY_EXISTS, got %v", err)
	}
	if err := s.Save(context.Background(), specIdentity(t)); !shared.IsCode(err, shared.CodeIdentityExists) {
		t.Fatalf("Save: want IDENTITY_EXISTS, got %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "keep" {
		t.Fatalf("file overwritten: %q", b)
	}
}

func TestSave_DashWritesToStdout(t *testing.T) {
	var out bytes.Buffer
	s, _ := envfile.New(envfile.Stdout, envfile.FormatEnv, envfile.WithStdout(&out))
	if err := s.Writable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(context.Background(), specIdentity(t)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "BUZZ_AUTH_TAG='"+specTag+"'") {
		t.Fatalf("stdout = %s", out.String())
	}
}

func TestRender_EnvQuotesValuesAndCommentsPublicFacts(t *testing.T) {
	b, err := envfile.Render(specIdentity(t), envfile.FormatEnv)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"BUZZ_RELAY_URL='https://relay.example'\n",
		"BUZZ_PRIVATE_KEY=nsec1",
		"BUZZ_AUTH_TAG='" + specTag + "'\n",
		"# agent npub1",
		"# owner npub1",
		"# expires 2024-04-24T11:10:00Z",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
}

func TestParse_AcceptsShellAndDotenvSpellings(t *testing.T) {
	in := "# comment\n" +
		"export BUZZ_RELAY_URL=\"wss://relay.example\"\n" +
		"BUZZ_PRIVATE_KEY = " + agentSK + "\n" +
		"OTHER=ignored\n" +
		"BUZZ_AUTH_TAG='" + specTag + "'\n"
	rec, err := envfile.Parse([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if rec.RelayURL != "wss://relay.example" || rec.PrivateKey != agentSK || rec.AuthTag != specTag {
		t.Fatalf("record = %+v", rec)
	}
}

func TestParse_RequiresRelayAndKey(t *testing.T) {
	for _, in := range []string{"", "BUZZ_RELAY_URL=x\n", `{"BUZZ_PRIVATE_KEY":"x"}`, `{"BUZZ_RELAY_URL":1}`} {
		if _, err := envfile.Parse([]byte(in)); !shared.IsCode(err, shared.CodeIdentityInvalid) {
			t.Errorf("%q: want IDENTITY_INVALID, got %v", in, err)
		}
	}
}

func TestNew_RejectsUnknownFormat(t *testing.T) {
	if _, err := envfile.New("x", "yaml"); !shared.IsCode(err, shared.CodeIdentityInvalid) {
		t.Fatalf("want IDENTITY_INVALID, got %v", err)
	}
}

func TestRender_GrokPromptInstallsTheServerWithTheVariables(t *testing.T) {
	id := specIdentity(t)
	out, err := envfile.Render(id, envfile.FormatGrok)
	if err != nil {
		t.Fatal(err)
	}
	msg := string(out)
	for _, want := range []string{
		"npx -y github:davidorban/buzz-grokbot",
		"BUZZ_RELAY_URL = https://relay.example\n",
		"BUZZ_PRIVATE_KEY = " + id.Secret().Nsec() + "\n",
		"BUZZ_AUTH_TAG = " + specTag + "\n",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	if !regexp.MustCompile(`github:davidorban/buzz-grokbot#[0-9a-f]{40}\n`).MatchString(msg) {
		t.Errorf("MCP server is not pinned to a full commit:\n%s", msg)
	}
	if strings.Contains(msg, "'") {
		t.Errorf("message quotes a value; Grok Bot would keep the quotes:\n%s", msg)
	}

	member, _ := identity.NewMember(id.Relay(), id.Secret(), id.Agent())
	out, _ = envfile.Render(member, envfile.FormatGrok)
	if strings.Contains(string(out), "BUZZ_AUTH_TAG") {
		t.Fatalf("member identity message names an auth tag:\n%s", out)
	}
}

func TestRemove_DeletesTheFileOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.env")
	s, _ := envfile.New(path, envfile.FormatEnv)
	if err := s.Save(context.Background(), specIdentity(t)); err != nil {
		t.Fatal(err)
	}
	if removed, err := s.Remove(context.Background()); !removed || err != nil {
		t.Fatalf("first remove: %v %v", removed, err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
	if removed, err := s.Remove(context.Background()); removed || err != nil {
		t.Fatalf("second remove: %v %v", removed, err)
	}
}
