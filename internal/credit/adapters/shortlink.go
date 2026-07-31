package adapters

import (
	"context"

	"github.com/Shamba-Records-Limited/microvault/pkg/urlshortener"
)

// shortenedLink returns the link to put in an SMS.
//
// dub is pointed at rawURL — the provider's own URL — rather than at fallback,
// the internal /r/{code} redirect. MoneyGram expires a settled session itself,
// so the redirect adds a hop without adding control, and shortening the real
// destination makes the link preview name it. fallback is returned when no
// shortener is configured, when there is no raw URL, or when dub fails, so a
// borrower always gets a link that resolves.
//
// A non-nil error means the shortener failed and fallback was used; callers log
// it with their own context.
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
