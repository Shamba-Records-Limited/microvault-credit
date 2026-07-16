package adapters

import (
	"crypto/rand"
	"encoding/base64"
)

// newShortCode returns a URL-safe, unguessable ~11-char code (64 bits) used as
// the bearer token in a /r/{code} SMS redirect link.
func newShortCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
