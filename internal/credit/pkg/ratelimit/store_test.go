package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestStore(t *testing.T) (*RedisStore, *miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisStore(client, "test:ratelimit"), mr, client
}

func TestGet_MissingKeyReturnsNilNil(t *testing.T) {
	// fiber.Storage requires nil, nil rather than an error, or the limiter
	// treats every first request as a backend failure.
	s, _, _ := newTestStore(t)

	val, err := s.Get("absent")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil for a missing key", err)
	}
	if val != nil {
		t.Errorf("Get() = %v, want nil", val)
	}
}

func TestSetGetDelete(t *testing.T) {
	s, _, _ := newTestStore(t)

	if err := s.Set("k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	val, err := s.Get("k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(val) != "v" {
		t.Errorf("Get() = %q, want %q", val, "v")
	}

	if err := s.Delete("k"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if val, _ := s.Get("k"); val != nil {
		t.Errorf("Get() after Delete = %q, want nil", val)
	}
}

func TestSet_ExpiryIsApplied(t *testing.T) {
	s, mr, _ := newTestStore(t)

	if err := s.Set("k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	mr.FastForward(2 * time.Minute)

	if val, _ := s.Get("k"); val != nil {
		t.Errorf("Get() past expiry = %q, want nil", val)
	}
}

func TestKeysAreNamespaced(t *testing.T) {
	s, _, client := newTestStore(t)

	if err := s.Set("k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if got := client.Get(context.Background(), "test:ratelimit:k").Val(); got != "v" {
		t.Errorf("key not written under the prefix; got %q", got)
	}
}

func TestReset_LeavesOtherKeysAlone(t *testing.T) {
	// The client is shared with USSD sessions and idempotency records, so Reset
	// must not behave like FLUSHDB.
	s, _, client := newTestStore(t)
	ctx := context.Background()

	if err := s.Set("mine", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := client.Set(ctx, "microvault:session:abc", "keep", time.Minute).Err(); err != nil {
		t.Fatalf("seeding a foreign key failed: %v", err)
	}

	if err := s.Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	if val, _ := s.Get("mine"); val != nil {
		t.Errorf("Reset() left its own key = %q, want nil", val)
	}
	if got := client.Get(ctx, "microvault:session:abc").Val(); got != "keep" {
		t.Errorf("Reset() destroyed a foreign key; got %q, want %q", got, "keep")
	}
}

func TestClose_DoesNotCloseSharedClient(t *testing.T) {
	// The cache package owns the client's lifecycle. If Close closed it here,
	// shutting down the limiter would drop every other Redis consumer.
	s, _, client := newTestStore(t)

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Errorf("shared client unusable after Close(): %v", err)
	}
}
