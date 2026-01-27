package config

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates a new Zap logger based on configuration.
func NewLogger(cfg *Config) (*zap.Logger, error) {
	var config zap.Config

	if strings.ToLower(cfg.LogFormat) == "console" {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		config = zap.NewProductionConfig()
		config.EncoderConfig.TimeKey = "ts"
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	// Set log level
	level, err := zapcore.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zapcore.InfoLevel
	}
	config.Level = zap.NewAtomicLevelAt(level)

	// Build logger
	logger, err := config.Build(
		zap.AddCallerSkip(0),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, err
	}

	// Replace global logger
	zap.ReplaceGlobals(logger)

	return logger, nil
}

// MustNewLogger creates a logger and panics on error (for initialization).
func MustNewLogger(cfg *Config) *zap.Logger {
	logger, err := NewLogger(cfg)
	if err != nil {
		os.Stderr.WriteString("failed to create logger: " + err.Error() + "\n")
		os.Exit(1)
	}
	return logger
}
