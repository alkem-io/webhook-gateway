// Package config provides typed configuration loading from environment variables.
package config

import (
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
		RedisURL:            getEnv("REDIS_URL", "redis://localhost:6379/0"),
		RabbitMQURL:         getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		PlatformURL:         getEnv("PLATFORM_URL", "https://alkem.io"),
		MaintenanceMode:     getEnvBool("MAINTENANCE_MODE", false),
		MaintenanceMessage:  getEnv("MAINTENANCE_MESSAGE", "Service under maintenance"),
		CorrelationIDHeader: getEnv("CORRELATION_ID_HEADER", "X-Request-ID"),
	}

	return cfg, nil
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
