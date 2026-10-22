// Package fakerelay is an in-process Buzz relay for tests. It enforces the
// same admission rules as Buzz's REST API: a valid, unreplayed NIP-98
// Authorization event for scheme://<host>/<route>, then relay membership, or
// (when NIP-OA admission is on) an x-auth-tag attestation by a member owner
// whose time conditions hold at the authentication event's created_at.
//
// It is stricter than Buzz in one respect: it requires the nonce and payload
// tags, which buzz-cli and gobuzzkey always send.
package fakerelay

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mariotoffia/gobuzzkey/adapters/crypto/secp256k1"
	"github.com/mariotoffia/gobuzzkey/domain/attestation"
	"github.com/mariotoffia/gobuzzkey/domain/clock"
	"github.com/mariotoffia/gobuzzkey/domain/key"
	"github.com/mariotoffia/gobuzzkey/domain/nostr"
)

// AuthSkew is how far a NIP-98 created_at may be from the relay clock.
const AuthSkew = 60 * time.Second

// Routes. Like Buzz, the relay serves them at the host root and expects the
// NIP-98 "u" tag to be scheme://<Host header><route>.
const (
	routeQuery  = "/query"
	routeEvents = "/events"
)

// Channel is a channel the fake relay serves. Like Buzz, an open channel is
// visible to every admitted caller and joinable with a kind-9021 request; a
// private one is visible to its members only. Only members may post.
type Channel struct {
	ID      string
	Name    string
	Private bool
	Members []key.PublicKey
}

// Request is a recorded request.
type Request struct {
	Path    string
	Auth    nostr.Event // decoded Authorization event
	AuthTag string      // raw x-auth-tag header
	Body    []byte
}

// Relay is the fake.
type Relay struct {
	mu        sync.Mutex
	seenAuth  map[string]bool // NIP-98 event IDs already used (replay guard)
	server    *httptest.Server
	clock     clock.Clock
	crypto    secp256k1.Crypto
	members   map[key.PublicKey]bool
	allowNIPO bool
	channels  []Channel
	messages  []nostr.Event
	requests  []Request
	status    int // when non-zero, every request answers this status
}

// Option configures the relay.
type Option func(*Relay)

// WithMembers enrolls relay members.
func WithMembers(keys ...key.PublicKey) Option {
	return func(r *Relay) {
		for _, k := range keys {
			r.members[k] = true
		}
	}
}

// WithNIPOA turns NIP-OA owner-attestation admission on or off (Buzz's
// BUZZ_ALLOW_NIP_OA_AUTH, default off).
func WithNIPOA(on bool) Option { return func(r *Relay) { r.allowNIPO = on } }

// WithChannels adds channels.
func WithChannels(chs ...Channel) Option {
	return func(r *Relay) { r.channels = append(r.channels, chs...) }
}

// WithClock sets the relay clock.
func WithClock(clk clock.Clock) Option { return func(r *Relay) { r.clock = clk } }

// WithStatus makes every request fail with status (to test error handling).
func WithStatus(status int) Option { return func(r *Relay) { r.status = status } }

// Start runs the relay until the returned Close is called.
func Start(opts ...Option) *Relay {
	r := &Relay{members: map[key.PublicKey]bool{}, seenAuth: map[string]bool{}, clock: clock.Real{}, crypto: secp256k1.New()}
	for _, o := range opts {
		o(r)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+routeQuery, r.handleQuery)
	mux.HandleFunc("POST "+routeEvents, r.handleEvents)
	r.server = httptest.NewServer(mux)
	return r
}

// URL is the relay base URL.
func (r *Relay) URL() string { return r.server.URL }

// Close stops the relay.
func (r *Relay) Close() { r.server.Close() }

// Requests returns a copy of the recorded requests.
func (r *Relay) Requests() []Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Request(nil), r.requests...)
}

// Members returns a copy of the members of channel id.
func (r *Relay) Members(id string) []key.PublicKey {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ch := r.channel(id); ch != nil {
		return append([]key.PublicKey(nil), ch.Members...)
	}
	return nil
}

// Messages returns a copy of the accepted kind-9 events.
func (r *Relay) Messages() []nostr.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]nostr.Event(nil), r.messages...)
}

func (r *Relay) handleQuery(w http.ResponseWriter, req *http.Request) {
	caller, body, ok := r.admit(w, req)
	if !ok {
		return
	}
	var filters []map[string]json.RawMessage
	if err := json.Unmarshal(body, &filters); err != nil || len(filters) != 1 {
		writeError(w, http.StatusBadRequest, "invalid_filter", "expected one filter")
		return
	}
	var kinds []int
	_ = json.Unmarshal(filters[0]["kinds"], &kinds)
	var hs, ds []string
	_ = json.Unmarshal(filters[0]["#h"], &hs)
	_ = json.Unmarshal(filters[0]["#d"], &ds)

	r.mu.Lock()
	defer r.mu.Unlock()
	out := []nostr.Event{}
	for _, k := range kinds {
		switch k {
		case nostr.KindChannelMetadata:
			for _, ch := range r.channels {
				if ch.visibleTo(caller) {
					out = append(out, nostr.Event{Kind: k, Tags: [][]string{{"d", ch.ID}, {"name", ch.Name}, {ch.visibility()}}})
				}
			}
		case nostr.KindChannelMembers:
			for _, ch := range r.channels {
				if contains(ds, ch.ID) && ch.visibleTo(caller) {
					ev := nostr.Event{Kind: k, Tags: [][]string{{"d", ch.ID}}}
					for _, m := range ch.Members {
						ev.Tags = append(ev.Tags, []string{"p", m.Hex(), "member"})
					}
					out = append(out, ev)
				}
			}
		case nostr.KindChannelMessage:
			for _, m := range r.messages {
				if h, _ := m.TagValue("h"); contains(hs, h) && r.channelVisible(h, caller) {
					out = append(out, m)
				}
			}
		}
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (r *Relay) handleEvents(w http.ResponseWriter, req *http.Request) {
	caller, body, ok := r.admit(w, req)
	if !ok {
		return
	}
	var ev nostr.Event
	if err := json.Unmarshal(body, &ev); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_event", err.Error())
		return
	}
	if msg := r.validEvent(ev); msg != "" {
		writeError(w, http.StatusBadRequest, "invalid_event", msg)
		return
	}
	h, _ := ev.TagValue("h")
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := r.channel(h)
	switch {
	case ev.PubKey != caller.Hex():
		writeResult(w, ev.ID, false, "restricted: event author is not the authenticated caller")
	case ch == nil:
		writeResult(w, ev.ID, false, "invalid: channel not found")
	case ev.Kind == nostr.KindJoinRequest && ch.Private && !ch.isMember(caller):
		writeResult(w, ev.ID, false, "restricted: channel is private")
	case ev.Kind == nostr.KindJoinRequest:
		if !ch.isMember(caller) {
			ch.Members = append(ch.Members, caller)
		}
		writeResult(w, ev.ID, true, "")
	case ev.Kind == nostr.KindLeaveRequest:
		ch.Members = slices.DeleteFunc(ch.Members, func(k key.PublicKey) bool { return k == caller })
		writeResult(w, ev.ID, true, "")
	case !ch.isMember(caller):
		writeResult(w, ev.ID, false, "restricted: not a channel member")
	default:
		r.messages = append(r.messages, ev)
		writeResult(w, ev.ID, true, "")
	}
}

// admit verifies NIP-98 and membership. It writes the error response itself.
func (r *Relay) admit(w http.ResponseWriter, req *http.Request) (key.PublicKey, []byte, bool) {
	body, _ := io.ReadAll(req.Body)
	auth, authErr := r.verifyHTTPAuth(req, body)
	r.mu.Lock()
	r.requests = append(r.requests, Request{Path: req.URL.Path, Auth: auth, AuthTag: req.Header.Get("x-auth-tag"), Body: body})
	status := r.status
	r.mu.Unlock()

	if status != 0 {
		writeError(w, status, "forced", "forced status")
		return key.PublicKey{}, nil, false
	}
	if authErr != "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", authErr)
		return key.PublicKey{}, nil, false
	}
	caller, _ := key.ParsePublicKeyHex(auth.PubKey)
	if r.isMember(caller) || r.admittedByOwner(caller, req.Header.Values("x-auth-tag"), auth.CreatedAt) {
		return caller, body, true
	}
	writeError(w, http.StatusForbidden, "relay_membership_required", "You must be a relay member to access this relay")
	return key.PublicKey{}, nil, false
}

func (r *Relay) verifyHTTPAuth(req *http.Request, body []byte) (nostr.Event, string) {
	raw, ok := strings.CutPrefix(req.Header.Get("Authorization"), nostr.AuthorizationScheme)
	if !ok {
		return nostr.Event{}, "missing Nostr auth"
	}
	js, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nostr.Event{}, "auth is not base64"
	}
	var ev nostr.Event
	if err := json.Unmarshal(js, &ev); err != nil {
		return nostr.Event{}, "auth is not an event"
	}
	if msg := r.validEvent(ev); msg != "" {
		return ev, msg
	}
	if ev.Kind != nostr.KindHTTPAuth {
		return ev, "auth kind must be 27235"
	}
	url := "http://" + req.Host + req.URL.Path
	if u, _ := ev.TagValue("u"); u != url {
		return ev, "u tag " + u + " != " + url
	}
	if m, _ := ev.TagValue("method"); m != req.Method {
		return ev, "method tag mismatch"
	}
	sum := sha256.Sum256(body)
	if p, _ := ev.TagValue("payload"); p != hex.EncodeToString(sum[:]) {
		return ev, "payload tag mismatch"
	}
	if n, _ := ev.TagValue("nonce"); n == "" {
		return ev, "nonce tag missing"
	}
	skew := r.clock.Now().Sub(time.Unix(ev.CreatedAt, 0))
	if skew > AuthSkew || skew < -AuthSkew {
		return ev, "auth created_at outside the allowed skew"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seenAuth[ev.ID] {
		return ev, "auth event replayed"
	}
	r.seenAuth[ev.ID] = true
	return ev, ""
}

func (r *Relay) validEvent(ev nostr.Event) string {
	id := ev.ComputeID()
	if hex.EncodeToString(id[:]) != ev.ID {
		return "event id mismatch"
	}
	pub, err := key.ParsePublicKeyHex(ev.PubKey)
	if err != nil {
		return "bad pubkey"
	}
	sig, err := key.ParseSignatureHex(ev.Sig)
	if err != nil || !r.crypto.Verify(pub, id, sig) {
		return "bad signature"
	}
	return ""
}

func (r *Relay) admittedByOwner(agent key.PublicKey, tags []string, authCreatedAt int64) bool {
	if !r.allowNIPO || len(tags) != 1 {
		return false
	}
	tag, err := attestation.ParseAuthTag(tags[0])
	if err != nil {
		return false
	}
	att, err := attestation.New(agent, tag)
	if err != nil || !r.crypto.Verify(att.Owner(), att.Message(), tag.Sig) {
		return false
	}
	return tag.Conditions.AllowsAdmission(authCreatedAt) && r.isMember(tag.Owner)
}

func (r *Relay) isMember(k key.PublicKey) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.members[k]
}

// channel must be called with r.mu held.
func (r *Relay) channel(id string) *Channel {
	for i := range r.channels {
		if r.channels[i].ID == id {
			return &r.channels[i]
		}
	}
	return nil
}

// channelVisible must be called with r.mu held.
func (r *Relay) channelVisible(id string, caller key.PublicKey) bool {
	ch := r.channel(id)
	return ch != nil && ch.visibleTo(caller)
}

func (ch Channel) visibleTo(k key.PublicKey) bool { return !ch.Private || ch.isMember(k) }

func (ch Channel) isMember(k key.PublicKey) bool { return slices.Contains(ch.Members, k) }

// visibility is the kind-39000 flag tag Buzz publishes.
func (ch Channel) visibility() string {
	if ch.Private {
		return "private"
	}
	return "public"
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": msg})
}

func writeResult(w http.ResponseWriter, id string, accepted bool, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"event_id": id, "accepted": accepted, "message": msg})
}
