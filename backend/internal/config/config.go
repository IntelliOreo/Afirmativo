// Package config defines the application configuration struct,
// loads values from environment variables, and validates them at startup.
// Fails fast on missing or invalid configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all application configuration loaded from environment variables.
// In local dev, values come from .env via godotenv.
// In containers, values come from the runtime environment (e.g., Secret Manager).
type Config struct {
	Port               string
	FrontendURL        string
	DatabaseURL        string
	SessionExpiryHours int
}

// Load reads required environment variables and returns a validated Config.
// Returns an error if any required variable is missing.
func Load() (Config, error) {
	expiryStr := envOr("SESSION_EXPIRY_HOURS", "24")
	expiry, err := strconv.Atoi(expiryStr)
	if err != nil {
		return Config{}, fmt.Errorf("invalid SESSION_EXPIRY_HOURS: %w", err)
	}

	cfg := Config{
		Port:               envOr("PORT", "8080"),
		FrontendURL:        envOr("FRONTEND_URL", "http://localhost:3000"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		SessionExpiryHours: expiry,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
