package adapters

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryOnNotFound(t *testing.T) {
	notFound := errors.New("not found")
	otherErr := errors.New("something else broke")
	isRace := func(err error) bool { return errors.Is(err, notFound) }

	t.Run("succeeds immediately, no wait", func(t *testing.T) {
		calls := 0
		var waited []time.Duration
		got, err := retryOnNotFound(
			func() (string, error) { calls++; return "loan-1", nil },
			isRace,
			func(d time.Duration) { waited = append(waited, d) },
		)
		require.NoError(t, err)
		assert.Equal(t, "loan-1", got)
		assert.Equal(t, 1, calls)
		assert.Empty(t, waited, "must not wait when the first attempt succeeds")
	})

	t.Run("retries a race error and succeeds once the row appears", func(t *testing.T) {
		calls := 0
		var waited []time.Duration
		got, err := retryOnNotFound(
			func() (string, error) {
				calls++
				if calls < 3 {
					return "", notFound
				}
				return "loan-1", nil
			},
			isRace,
			func(d time.Duration) { waited = append(waited, d) },
		)
		require.NoError(t, err)
		assert.Equal(t, "loan-1", got)
		assert.Equal(t, 3, calls)
		assert.Len(t, waited, 2, "waits between attempts, not after the last one")
	})

	t.Run("gives up after disbursementLookupRetries attempts", func(t *testing.T) {
		calls := 0
		got, err := retryOnNotFound(
			func() (string, error) { calls++; return "", notFound },
			isRace,
			func(time.Duration) {},
		)
		assert.ErrorIs(t, err, notFound)
		assert.Empty(t, got)
		assert.Equal(t, disbursementLookupRetries, calls)
	})

	t.Run("a non-race error returns immediately without retrying", func(t *testing.T) {
		calls := 0
		_, err := retryOnNotFound(
			func() (string, error) { calls++; return "", otherErr },
			isRace,
			func(time.Duration) { t.Fatal("must not wait on a non-race error") },
		)
		assert.ErrorIs(t, err, otherErr)
		assert.Equal(t, 1, calls, "a real failure must not consume the race retry budget")
	})
}
