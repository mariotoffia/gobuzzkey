// Package buzzhttp implements ports.Relay over Buzz's REST API: POST /query
// and POST /events, each signed with NIP-98 as the agent and carrying the
// owner attestation in the x-auth-tag header (Buzz NIP-AA admission).
package buzzhttp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mariotoffia/gobuzzkey/domain/clock"
	"github.com/mariotoffia/gobuzzkey/domain/identity"
	"github.com/mariotoffia/gobuzzkey/domain/nostr"
	"github.com/mariotoffia/gobuzzkey/domain/relay"
	"github.com/mariotoffia/gobuzzkey/domain/shared"
	"github.com/mariotoffia/gobuzzkey/ports"
)

// HeaderAuthTag carries the NIP-OA tag JSON to the relay.
const HeaderAuthTag = "x-auth-tag"

// errorMembershipRequired is the error string Buzz returns with 403 when the
// caller is neither a member nor attested by one.
const errorMembershipRequired = "relay_membership_required"

const (
	defaultTimeout  = 30 * time.Second
	maxResponseSize = 16 << 20
	maxErrorText    = 300
	nonceBytes      = 16
)

// Client is an authenticated session with one Buzz relay.
type Client struct {
	identity identity.AgentIdentity
	authTag  string
	signer   ports.Signer
	http     *http.Client
	clock    clock.Clock
	nonce    func() (string, error)
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the default client (30 s timeout).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithClock sets the clock used for NIP-98 created_at.
func WithClock(clk clock.Clock) Option { return func(c *Client) { c.clock = clk } }

// WithNonce replaces the random NIP-98 nonce source.
func WithNonce(f func() (string, error)) Option { return func(c *Client) { c.nonce = f } }

// New returns a client acting as id. The attestation, when present, is sent
// on every request.
func New(id identity.AgentIdentity, signer ports.Signer, opts ...Option) *Client {
	c := &Client{
		identity: id,
		signer:   signer,
		http:     &http.Client{Timeout: defaultTimeout},
		clock:    clock.Real{},
		nonce:    randomNonce,
	}
	if att, ok := id.Attestation(); ok {
		c.authTag = att.Tag().String()
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Query runs one NIP-01 filter against /query.
func (c *Client) Query(ctx context.Context, filter nostr.Filter) ([]nostr.Event, error) {
	f, err := filter.MarshalJSON()
	if err != nil {
		return nil, err
	}
	body := append(append([]byte{'['}, f...), ']')
	raw, err := c.post(ctx, "/query", body)
	if err != nil {
		return nil, err
	}
	var events []nostr.Event
	if err := json.Unmarshal(raw, &events); err != nil {
		return nil, shared.Wrap(shared.CodeRelayRejected, "relay /query did not return an event array", err)
	}
	return events, nil
}

// Publish submits a signed event to /events.
func (c *Client) Publish(ctx context.Context, ev nostr.Event) (relay.WriteResult, error) {
	body, err := ev.Marshal()
	if err != nil {
		return relay.WriteResult{}, err
	}
	raw, err := c.post(ctx, "/events", body)
	if err != nil {
		return relay.WriteResult{}, err
	}
	var res struct {
		EventID  string `json:"event_id"`
		Accepted bool   `json:"accepted"`
		Message  string `json:"message"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return relay.WriteResult{}, shared.Wrap(shared.CodeRelayRejected, "relay /events returned an unexpected body", err)
	}
	return relay.WriteResult{EventID: res.EventID, Accepted: res.Accepted, Message: res.Message}, nil
}

func (c *Client) post(ctx context.Context, path string, body []byte) ([]byte, error) {
	url := c.identity.Relay().Endpoint(path)
	auth, err := c.authorization(url, body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, shared.Wrap(shared.CodeRelayUnavailable, "build request", err)
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	if c.authTag != "" {
		req.Header.Set(HeaderAuthTag, c.authTag)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, shared.Wrap(shared.CodeRelayUnavailable, "POST "+url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, shared.Wrap(shared.CodeRelayUnavailable, "read "+url, err)
	}
	if resp.StatusCode >= 300 {
		return nil, classify(path, resp.StatusCode, raw)
	}
	return raw, nil
}

// authorization builds "Nostr <base64(kind-27235 event)>".
func (c *Client) authorization(url string, body []byte) (string, error) {
	nonce, err := c.nonce()
	if err != nil {
		return "", shared.Wrap(shared.CodeRelayUnavailable, "NIP-98 nonce", err)
	}
	u := nostr.NewUnsigned(c.identity.Agent(), c.clock.Now(), nostr.KindHTTPAuth,
		nostr.HTTPAuthTags(url, http.MethodPost, body, nonce), "")
	sig, err := c.signer.Sign(c.identity.Secret(), u.ID())
	if err != nil {
		return "", err
	}
	ev, err := u.Signed(sig).Marshal()
	if err != nil {
		return "", err
	}
	return nostr.AuthorizationScheme + base64.StdEncoding.EncodeToString(ev), nil
}

func classify(path string, status int, body []byte) error {
	text := errorText(body)
	msg := "relay " + path + ": HTTP " + strconv.Itoa(status) + ": " + text
	switch {
	case status == http.StatusForbidden && strings.Contains(text, errorMembershipRequired):
		return shared.New(shared.CodeRelayMembershipRequired, msg)
	case status == http.StatusUnauthorized:
		return shared.New(shared.CodeRelayUnauthorized, msg)
	case status >= 500:
		return shared.New(shared.CodeRelayUnavailable, msg)
	default:
		return shared.New(shared.CodeRelayRejected, msg)
	}
}

// errorText extracts Buzz's {"error": "...", "message": "..."} or falls back
// to the body, made terminal-safe and capped: it ends up in error messages.
func errorText(body []byte) string {
	var e struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	text := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &e) == nil && (e.Error != "" || e.Message != "") {
		text = strings.TrimSpace(strings.Join(nonEmpty(e.Error, e.Message), ": "))
	}
	return shared.Printable(text, maxErrorText)
}

func nonEmpty(values ...string) []string {
	out := values[:0]
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func randomNonce() (string, error) {
	b := make([]byte, nonceBytes)
	if _, err := rand.Read(b); err != nil {
		return "", errors.Join(errors.New("read random nonce"), err)
	}
	return hex.EncodeToString(b), nil
}

var _ ports.Relay = (*Client)(nil)
