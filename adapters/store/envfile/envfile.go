// Package envfile stores agent identities as an env file or a JSON object
// with the variable names buzz-cli, buzz-acp and Buzz MCP servers read:
// BUZZ_RELAY_URL, BUZZ_PRIVATE_KEY and BUZZ_AUTH_TAG.
package envfile

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/mariotoffia/gobuzzkey/domain/identity"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
	"github.com/mariotoffia/gobuzzkey/ports"
)

// Variable names.
const (
	VarRelayURL   = "BUZZ_RELAY_URL"
	VarPrivateKey = "BUZZ_PRIVATE_KEY"
	VarAuthTag    = "BUZZ_AUTH_TAG"
)

// Stdout is the path that writes to the configured writer instead of a file.
const Stdout = "-"

// fileMode keeps the identity readable by its owner only.
const fileMode fs.FileMode = 0o600

// Format selects the encoding.
type Format string

// Formats.
const (
	// FormatEnv is shell-sourceable KEY=VALUE lines. The relay URL and the
	// auth tag are single-quoted, so `source` and dotenv loaders keep the
	// tag's double quotes and nothing in a value is ever shell syntax.
	FormatEnv Format = "env"
	// FormatJSON is one JSON object, usable as an MCP server "env" block.
	FormatJSON Format = "json"
)

// Store reads and writes one identity location.
type Store struct {
	path   string
	format Format
	stdout io.Writer
}

// Option configures a Store.
type Option func(*Store)

// WithStdout sets where path "-" writes.
func WithStdout(w io.Writer) Option { return func(s *Store) { s.stdout = w } }

// New returns a store for path in format.
func New(path string, format Format, opts ...Option) (*Store, error) {
	if format != FormatEnv && format != FormatJSON {
		return nil, shared.New(shared.CodeIdentityInvalid, "format must be env or json, got "+string(format))
	}
	s := &Store{path: path, format: format, stdout: io.Discard}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

// Writable fails with IDENTITY_EXISTS when the destination file exists.
func (s *Store) Writable(_ context.Context) error {
	if s.path == Stdout {
		return nil
	}
	if _, err := os.Lstat(s.path); err == nil {
		return shared.New(shared.CodeIdentityExists, s.path+" already exists; choose another path or move it away")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return shared.Wrap(shared.CodeIdentityInvalid, "stat "+s.path, err)
	}
	return nil
}

// Save writes id. A file is created with mode 0600 and never overwritten.
func (s *Store) Save(_ context.Context, id identity.AgentIdentity) error {
	data, err := Render(id, s.format)
	if err != nil {
		return err
	}
	if s.path == Stdout {
		if _, err := s.stdout.Write(data); err != nil {
			return shared.Wrap(shared.CodeIdentityInvalid, "write identity", err)
		}
		return nil
	}
	f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
	if errors.Is(err, fs.ErrExist) {
		return shared.Wrap(shared.CodeIdentityExists, s.path+" already exists; choose another path or move it away", err)
	}
	if err != nil {
		return shared.Wrap(shared.CodeIdentityInvalid, "create "+s.path, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(s.path)
		return shared.Wrap(shared.CodeIdentityInvalid, "write "+s.path, err)
	}
	if err := f.Close(); err != nil {
		return shared.Wrap(shared.CodeIdentityInvalid, "close "+s.path, err)
	}
	return nil
}

// Load reads the identity, detecting env or JSON by content.
func (s *Store) Load(_ context.Context) (ports.IdentityRecord, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return ports.IdentityRecord{}, shared.Wrap(shared.CodeIdentityInvalid, "read "+s.path, err)
	}
	return Parse(data)
}

// Render encodes id in format.
func Render(id identity.AgentIdentity, format Format) ([]byte, error) {
	vars := [][2]string{
		{VarRelayURL, id.Relay().String()},
		{VarPrivateKey, id.Secret().Nsec()},
	}
	att, attested := id.Attestation()
	if attested {
		vars = append(vars, [2]string{VarAuthTag, att.Tag().String()})
	}
	if format == FormatJSON {
		m := make(map[string]string, len(vars))
		for _, kv := range vars {
			m[kv[0]] = kv[1]
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(m); err != nil {
			return nil, shared.Wrap(shared.CodeIdentityInvalid, "encode identity", err)
		}
		return buf.Bytes(), nil
	}
	var b strings.Builder
	b.WriteString("# gobuzzkey agent identity. Secret: anyone with this file can act as the agent.\n")
	b.WriteString("# agent " + id.Agent().Npub() + "\n")
	if attested {
		b.WriteString("# owner " + att.Owner().Npub() + "\n")
		if exp, ok := att.Expiry(); ok {
			b.WriteString("# expires " + exp.Format("2006-01-02T15:04:05Z") + "\n")
		}
	}
	for _, kv := range vars {
		v := kv[1]
		if kv[0] != VarPrivateKey { // an nsec is bech32: [a-z0-9] only
			v = "'" + v + "'"
		}
		b.WriteString(kv[0] + "=" + v + "\n")
	}
	return []byte(b.String()), nil
}

// Parse decodes an env file or a JSON object.
func Parse(data []byte) (ports.IdentityRecord, error) {
	vars := map[string]string{}
	trimmed := bytes.TrimSpace(data)
	if bytes.HasPrefix(trimmed, []byte("{")) {
		if err := json.Unmarshal(trimmed, &vars); err != nil {
			return ports.IdentityRecord{}, shared.Wrap(shared.CodeIdentityInvalid, "identity JSON must be an object of strings", err)
		}
	} else {
		parseEnv(data, vars)
	}
	rec := ports.IdentityRecord{
		RelayURL:   vars[VarRelayURL],
		PrivateKey: vars[VarPrivateKey],
		AuthTag:    vars[VarAuthTag],
	}
	if rec.RelayURL == "" || rec.PrivateKey == "" {
		return ports.IdentityRecord{}, shared.New(shared.CodeIdentityInvalid,
			"identity needs "+VarRelayURL+" and "+VarPrivateKey)
	}
	return rec, nil
}

func parseEnv(data []byte, vars map[string]string) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		vars[strings.TrimSpace(k)] = unquote(strings.TrimSpace(v))
	}
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '\'' || v[0] == '"') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

var (
	_ ports.IdentityStore  = (*Store)(nil)
	_ ports.IdentitySource = (*Store)(nil)
)
