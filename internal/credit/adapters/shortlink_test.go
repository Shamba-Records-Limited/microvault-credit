package adapters

import (
	"context"
	"errors"
	"testing"
)

// fakeShortener records what it was asked to shorten so tests can assert dub is
// pointed at the provider URL, not the internal redirect.
type fakeShortener struct {
	short  string
	err    error
	gotURL string
	calls  int
}

func (f *fakeShortener) Shorten(_ context.Context, longURL string) (string, error) {
	f.gotURL = longURL
	f.calls++
	return f.short, f.err
}

const (
	rawMGURL     = "https://extstellar.moneygram.com/?transaction_id=abc&token=JWT"
	redirectLink = "https://microvault.outray.app/r/RaMC6tg"
)

func TestShortenedLink_ShortensRawProviderURL(t *testing.T) {
	// The whole point of the change: dub must receive the MoneyGram URL so the
	// link preview names the real destination, not the /r/{code} hop.
	sh := &fakeShortener{short: "https://shmb.us/ab1"}

	got, err := shortenedLink(context.Background(), sh, rawMGURL, redirectLink)
	if err != nil {
		t.Fatalf("shortenedLink() error = %v", err)
	}
	if sh.gotURL != rawMGURL {
		t.Errorf("shortener got %q, want the raw MoneyGram URL", sh.gotURL)
	}
	if got != "https://shmb.us/ab1" {
		t.Errorf("link = %q, want the short link", got)
	}
}

func TestShortenedLink_Fallbacks(t *testing.T) {
	for name, tc := range map[string]struct {
		shortener *fakeShortener
		rawURL    string
		wantCalls int
	}{
		"shortener fails":    {shortener: &fakeShortener{err: errors.New("dub down")}, rawURL: rawMGURL, wantCalls: 1},
		"no raw URL":         {shortener: &fakeShortener{short: "https://shmb.us/ab1"}, rawURL: "", wantCalls: 0},
		"shortener disabled": {shortener: nil, rawURL: rawMGURL, wantCalls: 0},
	} {
		t.Run(name, func(t *testing.T) {
			// A nil *fakeShortener must be passed as a nil interface, not a
			// typed nil, or the disabled case would call through it.
			got, err := func() (string, error) {
				if tc.shortener == nil {
					return shortenedLink(context.Background(), nil, tc.rawURL, redirectLink)
				}
				return shortenedLink(context.Background(), tc.shortener, tc.rawURL, redirectLink)
			}()

			if got != redirectLink {
				t.Errorf("link = %q, want the fallback redirect", got)
			}
			if tc.shortener != nil {
				if tc.shortener.calls != tc.wantCalls {
					t.Errorf("shortener calls = %d, want %d", tc.shortener.calls, tc.wantCalls)
				}
				if tc.shortener.err != nil && err == nil {
					t.Error("a shortener failure must be reported so the caller can log it")
				}
			}
		})
	}
}
