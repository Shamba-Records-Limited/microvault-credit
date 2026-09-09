package adapters

import (
	"context"

	"github.com/Shamba-Records-Limited/microvault/pkg/urlshortener"
)

// shortenedLink returns the link to put in an SMS.
func shortenedLink(ctx context.Context, sh urlshortener.Shortener, rawURL, fallback string) (string, error) {
	if sh == nil || rawURL == "" {
		return fallback, nil
	}
	short, err := sh.Shorten(ctx, rawURL)
	if err != nil {
		return fallback, err
	}
	return short, nil
}
