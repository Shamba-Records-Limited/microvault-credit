// Package ratelimit adapts the shared Redis client to fiber.Storage so rate
// limits hold across replicas instead of per process.
package ratelimit

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// RedisStore implements [fiber.Storage] on top of an existing *redis.Client.
type RedisStore struct {
	client *redis.Client
	prefix string
}

var _ fiber.Storage = (*RedisStore)(nil)

// NewRedisStore wraps client, namespacing every key under prefix.
func NewRedisStore(client *redis.Client, prefix string) *RedisStore {
	return &RedisStore{client: client, prefix: prefix}
}

func (s *RedisStore) key(k string) string { return s.prefix + ":" + k }

// Get returns nil, nil for a missing key, as fiber.Storage requires.
func (s *RedisStore) Get(key string) ([]byte, error) {
	if key == "" {
		return nil, nil
	}
	val, err := s.client.Get(context.Background(), s.key(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return val, nil
}

// Set stores val under key. An exp of 0 means no expiration.
func (s *RedisStore) Set(key string, val []byte, exp time.Duration) error {
	if key == "" || len(val) == 0 {
		return nil
	}
	return s.client.Set(context.Background(), s.key(key), val, exp).Err()
}

func (s *RedisStore) Delete(key string) error {
	if key == "" {
		return nil
	}
	return s.client.Del(context.Background(), s.key(key)).Err()
}

// Reset deletes this store's keys only. fiber.Storage documents Reset as
// "delete all keys", but the client is shared — flushing the database would
// take USSD sessions and idempotency records with it.
func (s *RedisStore) Reset() error {
	ctx := context.Background()
	iter := s.client.Scan(ctx, 0, s.prefix+":*", 100).Iterator()
	for iter.Next(ctx) {
		if err := s.client.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}

// Close is a no-op: the client's lifecycle belongs to the cache package, which
// closes it on shutdown. Closing it here would drop every other Redis consumer.
func (s *RedisStore) Close() error { return nil }
