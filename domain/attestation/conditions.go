// Package attestation is the core of gobuzzkey: NIP-OA owner attestations.
//
// An owner key authorizes an agent key by signing
// SHA256("nostr:agent-auth:" || agent-pubkey-hex || ":" || conditions)
// with BIP-340. The result travels as the tag
// ["auth", <owner-pubkey-hex>, <conditions>, <sig-hex>].
package attestation

import (
	"strconv"
	"strings"
	"time"

	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// Clause operators and names, exactly as NIP-OA spells them.
const (
	clauseKind      = "kind"
	clauseCreatedAt = "created_at"
	maxKind         = 65535
	maxTimestamp    = 4294967295
)

// Clause is one condition: kind=<n>, created_at<t or created_at>t.
type Clause struct {
	Name  string // "kind" or "created_at"
	Op    byte   // '=', '<' or '>'
	Value uint64
}

// String renders the clause in canonical form.
func (c Clause) String() string {
	return c.Name + string(c.Op) + strconv.FormatUint(c.Value, 10)
}

// Conditions is a validated NIP-OA conditions string. The zero value is the
// empty string, which imposes no constraint. The original text is kept
// verbatim because clause order is part of the signed preimage.
type Conditions struct {
	raw     string
	clauses []Clause
}

// ParseConditions validates s against the NIP-OA grammar: empty, or clauses
// joined by '&' with canonical decimals, no whitespace and nothing else.
func ParseConditions(s string) (Conditions, error) {
	if s == "" {
		return Conditions{}, nil
	}
	parts := strings.Split(s, "&")
	clauses := make([]Clause, 0, len(parts))
	for _, part := range parts {
		c, err := parseClause(part)
		if err != nil {
			return Conditions{}, err
		}
		clauses = append(clauses, c)
	}
	return Conditions{raw: s, clauses: clauses}, nil
}

func parseClause(part string) (Clause, error) {
	invalid := func(why string) error {
		return shared.New(shared.CodeInvalidConditions, strconv.Quote(part)+": "+why)
	}
	if part == "" {
		return Clause{}, invalid("empty clause")
	}
	i := strings.IndexAny(part, "=<>")
	if i <= 0 {
		return Clause{}, invalid("unsupported clause")
	}
	c := Clause{Name: part[:i], Op: part[i]}
	limit := uint64(0)
	switch {
	case c.Name == clauseKind && c.Op == '=':
		limit = maxKind
	case c.Name == clauseCreatedAt && (c.Op == '<' || c.Op == '>'):
		limit = maxTimestamp
	default:
		return Clause{}, invalid("unsupported clause")
	}
	v, why := canonicalDecimal(part[i+1:], limit)
	if why != "" {
		return Clause{}, invalid(why)
	}
	c.Value = v
	return c, nil
}

// canonicalDecimal parses base-10 digits with no sign and no leading zero
// (except "0" itself) up to limit. It returns a reason when s is not one.
func canonicalDecimal(s string, limit uint64) (uint64, string) {
	if s == "" || (len(s) > 1 && s[0] == '0') {
		return 0, "number must be canonical decimal"
	}
	for j := range len(s) {
		if s[j] < '0' || s[j] > '9' {
			return 0, "number must be canonical decimal"
		}
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil || v > limit {
		return 0, "number out of range"
	}
	return v, ""
}

// Build creates conditions from clauses in the given order.
func Build(clauses ...Clause) (Conditions, error) {
	parts := make([]string, 0, len(clauses))
	for _, c := range clauses {
		parts = append(parts, c.String())
	}
	return ParseConditions(strings.Join(parts, "&"))
}

// Until returns a clause that limits the attestation to events created before t.
func Until(t time.Time) Clause {
	return Clause{Name: clauseCreatedAt, Op: '<', Value: uint64(max(t.Unix(), 0))}
}

// After returns a clause that limits the attestation to events created after t.
func After(t time.Time) Clause {
	return Clause{Name: clauseCreatedAt, Op: '>', Value: uint64(max(t.Unix(), 0))}
}

// Kind returns a clause that limits the attestation to one event kind.
func Kind(k uint16) Clause {
	return Clause{Name: clauseKind, Op: '=', Value: uint64(k)}
}

// String returns the conditions exactly as parsed.
func (c Conditions) String() string { return c.raw }

// IsEmpty reports whether there are no clauses.
func (c Conditions) IsEmpty() bool { return c.raw == "" }

// Clauses returns a copy of the clauses in signed order.
func (c Conditions) Clauses() []Clause { return append([]Clause(nil), c.clauses...) }

// Allows reports whether an event of kind created at createdAt satisfies every
// clause.
func (c Conditions) Allows(kind int, createdAt int64) bool {
	for _, cl := range c.clauses {
		if cl.Name == clauseKind && int64(kind) != int64(cl.Value) {
			return false
		}
		if !cl.allowsTime(createdAt) {
			return false
		}
	}
	return true
}

// AllowsAdmission is how a relay evaluates the attestation when it admits a
// connection (NIP-AA): created_at clauses are checked against the
// authentication event's time, kind clauses are not evaluated.
func (c Conditions) AllowsAdmission(createdAt int64) bool {
	for _, cl := range c.clauses {
		if !cl.allowsTime(createdAt) {
			return false
		}
	}
	return true
}

func (cl Clause) allowsTime(createdAt int64) bool {
	if cl.Name != clauseCreatedAt {
		return true
	}
	v := int64(cl.Value)
	if cl.Op == '<' {
		return createdAt < v
	}
	return createdAt > v
}

// Expiry returns the earliest created_at< bound: the first second the
// attestation no longer covers.
func (c Conditions) Expiry() (time.Time, bool) {
	var best uint64
	found := false
	for _, cl := range c.clauses {
		if cl.Name == clauseCreatedAt && cl.Op == '<' && (!found || cl.Value < best) {
			best, found = cl.Value, true
		}
	}
	if !found {
		return time.Time{}, false
	}
	return time.Unix(int64(best), 0).UTC(), true
}

// Kinds returns the event kinds the conditions name. An attestation with two
// different kind clauses allows no event at all.
func (c Conditions) Kinds() []uint16 {
	var kinds []uint16
	for _, cl := range c.clauses {
		if cl.Name == clauseKind {
			kinds = append(kinds, uint16(cl.Value))
		}
	}
	return kinds
}
