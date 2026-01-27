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

// IsWelcomeSent checks if the welcome_sent key exists for an identity.
// Returns true if already sent, false if not.
func (c *RedisClient) IsWelcomeSent(ctx context.Context, identityID string) (bool, error) {
	key := fmt.Sprintf("welcome_sent:%s", identityID)
	exists, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check welcome_sent key: %w", err)
	}
	return exists > 0, nil
}

// MarkWelcomeSent sets the welcome_sent key for an identity with TTL.
func (c *RedisClient) MarkWelcomeSent(ctx context.Context, identityID string) error {
	key := fmt.Sprintf("welcome_sent:%s", identityID)
	value := time.Now().UTC().Format(time.RFC3339)
	return c.client.Set(ctx, key, value, WelcomeSentTTL).Err()
}
