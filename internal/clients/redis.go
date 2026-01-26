// Package clients provides wrappers for external service clients.
package clients

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// WelcomeSentTTL is the TTL for welcome_sent keys (90 days).
	WelcomeSentTTL = 90 * 24 * time.Hour
)

// RedisClient wraps the Redis client with application-specific operations.
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient creates a new Redis client from the connection URL.
func NewRedisClient(url string) (*RedisClient, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	return &RedisClient{client: client}, nil
}

// Ping checks connectivity to Redis.
func (c *RedisClient) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Close closes the Redis connection.
func (c *RedisClient) Close() error {
	return c.client.Close()
}

// MarkWelcomeSentIfNew attempts to set the welcome_sent key for an identity.
// Returns true if the key was set (first time), false if it already exists.
func (c *RedisClient) MarkWelcomeSentIfNew(ctx context.Context, identityID string) (bool, error) {
	key := fmt.Sprintf("welcome_sent:%s", identityID)
	value := time.Now().UTC().Format(time.RFC3339)

	set, err := c.client.SetNX(ctx, key, value, WelcomeSentTTL).Result()
	if err != nil {
		return false, fmt.Errorf("failed to set welcome_sent key: %w", err)
	}

	return set, nil
}
