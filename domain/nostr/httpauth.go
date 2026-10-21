package nostr

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// HTTPAuthTags returns the NIP-98 tags for a request: u (absolute URL),
// method, nonce and, when body is non-nil, payload = SHA256(body) in hex.
// This matches what buzz-cli signs.
func HTTPAuthTags(url, method string, body []byte, nonce string) [][]string {
	tags := [][]string{
		{"u", url},
		{"method", strings.ToUpper(method)},
		{"nonce", nonce},
	}
	if body != nil {
		sum := sha256.Sum256(body)
		tags = append(tags, []string{"payload", hex.EncodeToString(sum[:])})
	}
	return tags
}

// AuthorizationScheme prefixes the base64 event in the Authorization header.
const AuthorizationScheme = "Nostr "
