package adapters

import (
	"crypto/rand"
	"encoding/base64"
)

// newShortCode returns a URL-safe, unguessable 7-char code (40 bits) used as
// the bearer token in a /r/{code} SMS redirect link.
//
// 40 bits resists guessing a specific code, not enumeration of the whole space:
// finding any live code costs roughly 2^40 divided by the number of live codes.
// The rate limit on /r/:code and [shortCodeTTL] are what make that impractical,
// so do not widen the code's use without them.
func newShortCode() (string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
