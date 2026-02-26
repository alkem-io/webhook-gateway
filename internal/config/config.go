// Package config provides typed configuration loading from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all application configuration.
type Config struct {
	// Server configuration
	Port      int
	LogLevel  string
	LogFormat string

	// Redis configuration
	RedisURL string

	// RabbitMQ configuration
	RabbitMQURL string

	// Platform configuration
	PlatformURL string

	// Maintenance mode
	MaintenanceMode    bool
	MaintenanceMessage string

	// Correlation ID header name
	CorrelationIDHeader string

	// Login backoff configuration
	LoginBackoffMaxIdentifierAttempts    int
	LoginBackoffMaxIPAttempts            int
	LoginBackoffIdentifierLockoutSeconds int
	LoginBackoffIPLockoutSeconds         int

	// Kratos internal URL for login proxy
	KratosInternalURL string
}

// Load reads configuration from environment variables.
// It loads .env file if present (for local development).
func Load() (*Config, error) {
	// Load .env file if it exists (ignore errors for production)
	_ = godotenv.Load()

	cfg := &Config{
		Port:                getEnvInt("PORT", 8080),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		LogFormat:           getEnv("LOG_FORMAT", "json"),
		RedisURL:            resolveRedisURL(),
		RabbitMQURL:         getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		PlatformURL:         getEnv("PLATFORM_URL", "https://alkem.io"),
		MaintenanceMode:     getEnvBool("MAINTENANCE_MODE", false),
		MaintenanceMessage:  getEnv("MAINTENANCE_MESSAGE", "Service under maintenance"),
		CorrelationIDHeader: getEnv("CORRELATION_ID_HEADER", "X-Request-ID"),

		LoginBackoffMaxIdentifierAttempts:    getEnvInt("LOGIN_BACKOFF_MAX_IDENTIFIER_ATTEMPTS", 10),
		LoginBackoffMaxIPAttempts:            getEnvInt("LOGIN_BACKOFF_MAX_IP_ATTEMPTS", 20),
		LoginBackoffIdentifierLockoutSeconds: getEnvInt("LOGIN_BACKOFF_IDENTIFIER_LOCKOUT_SECONDS", 120),
		LoginBackoffIPLockoutSeconds:         getEnvInt("LOGIN_BACKOFF_IP_LOCKOUT_SECONDS", 120),

		KratosInternalURL: getEnv("KRATOS_INTERNAL_URL", "http://kratos:4433"),
	}

	return cfg, nil
}

// resolveRedisURL returns the Redis connection URL.
// Prefers REDIS_URL if set; otherwise builds from REDIS_HOST + REDIS_PORT.
func resolveRedisURL() string {
	if url := os.Getenv("REDIS_URL"); url != "" {
		return url
	}
	host := getEnv("REDIS_HOST", "localhost")
	port := getEnv("REDIS_PORT", "6379")
	return fmt.Sprintf("redis://%s:%s/0", host, port)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
