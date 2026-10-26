package config

import (
	"encoding/hex"
	"errors"
	"os"
	"strings"
)

// Config contains validated application configuration.
type Config struct {
	DatabaseURL   string
	AdminAPIToken string
}

// Load reads and validates configuration from environment variables.
func Load() (Config, error) {
	cfg := Config{
		// Reading  configuration from the process environment
		// instead of hardcoding secrets
		DatabaseURL:   strings.TrimSpace(os.Getenv("DATABASE_URL")),
		AdminAPIToken: strings.TrimSpace(os.Getenv("ADMIN_API_TOKEN")),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL environment variable is not set")
	}

	// Checking if the admin secret is 64 hexadecimal characters.
	if len(cfg.AdminAPIToken) != 64 {
		return Config{}, errors.New("ADMIN_API_TOKEN must be 64 hexadecimal characters")
	}

	// Decoding the hex string to check that every character is valid.
	if _, err := hex.DecodeString(cfg.AdminAPIToken); err != nil {
		return Config{}, errors.New("ADMIN_API_TOKEN contains invalid hexadecimal characters")
	}

	return cfg, nil
}
