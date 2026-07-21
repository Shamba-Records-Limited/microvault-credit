package adapters

import (
	"crypto/rand"
	"encoding/base64"
)

// newShortCode returns a URL-safe, unguessable 7-char code (40 bits) used as
// the bearer token in a /r/{code} SMS redirect link. Kept short to fit SMS
// width; 40 bits is ample for a rate-limited, 24h-lived link.
func newShortCode() (string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
