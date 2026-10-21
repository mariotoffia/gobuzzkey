package nostr

import (
	"bytes"
	"encoding/json"

	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// Filter is a NIP-01 query filter.
type Filter struct {
	IDs     []string
	Authors []string
	Kinds   []int
	// Tags maps a single-letter tag name to accepted values; it is encoded as
	// "#<name>".
	Tags  map[string][]string
	Since int64 // 0 means unset
	Until int64 // 0 means unset
	Limit int   // 0 means unset
}

// MarshalJSON encodes the filter with only the set fields.
func (f Filter) MarshalJSON() ([]byte, error) {
	m := map[string]any{}
	if len(f.IDs) > 0 {
		m["ids"] = f.IDs
	}
	if len(f.Authors) > 0 {
		m["authors"] = f.Authors
	}
	if len(f.Kinds) > 0 {
		m["kinds"] = f.Kinds
	}
	for name, values := range f.Tags {
		m["#"+name] = values
	}
	if f.Since > 0 {
		m["since"] = f.Since
	}
	if f.Until > 0 {
		m["until"] = f.Until
	}
	if f.Limit > 0 {
		m["limit"] = f.Limit
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return nil, shared.Wrap(shared.CodeInvalidEvent, "encode filter", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
