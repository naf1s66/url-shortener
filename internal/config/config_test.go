package config

import (
	"strings"
	"testing"
)

// TestLoadValidConfig verifies that valid environment variables
// are loaded and returned correctly.
func TestLoadValidConfig(t *testing.T) {
	validDatabaseURL := "postgres://localhost/testdb"
	// Setenv() resets env value after test
	t.Setenv("DATABASE_URL", validDatabaseURL)
	validToken := strings.Repeat("a", 64)
	t.Setenv("ADMIN_API_TOKEN", validToken)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.DatabaseURL != validDatabaseURL {
		t.Errorf("expected database URL %q, got %q", validDatabaseURL, cfg.DatabaseURL)
	}
	if cfg.AdminAPIToken != validToken {
		t.Errorf("expected admin API token %q, got %q", validToken, cfg.AdminAPIToken)
	}
}

// TestLoadMissingAdminToken verifies that an empty admin token
// causes configuration loading to fail.
func TestLoadMissingAdminToken(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/testdb")
	missingToken := ""
	t.Setenv("ADMIN_API_TOKEN", missingToken)
	_, err := Load()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

// TestLoadMissingDatabaseURL verifies that an empty database URL
// causes configuration loading to fail.
func TestLoadMissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	validToken := strings.Repeat("a", 64)
	t.Setenv("ADMIN_API_TOKEN", validToken)
	_, err := Load()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

// TestLoadAdminTokenLength checks the 64-character token requirement
// using boundary values of 63, 64, and 65 characters.
func TestLoadAdminTokenLength(t *testing.T) {
	// Test cases
	tests := []struct {
		name    string
		length  int
		wantErr bool
	}{
		{
			name:    "too short",
			length:  63,
			wantErr: true,
		},
		{
			name:    "valid length",
			length:  64,
			wantErr: false},
		{
			name:    "too long",
			length:  65,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			validDatabaseURL := "postgres://localhost/testdb"
			t.Setenv("DATABASE_URL", validDatabaseURL)

			validToken := strings.Repeat("a", tc.length)
			t.Setenv("ADMIN_API_TOKEN", validToken)

			cfg, err := Load()

			if (err != nil) != tc.wantErr {
				t.Fatalf("expected error: %t, got error: %v", tc.wantErr, err)
			}
			if !tc.wantErr && cfg.AdminAPIToken != validToken {
				t.Errorf("expected admin API token %q, got %q", validToken, cfg.AdminAPIToken)
			}
		})
	}
}

// TestLoadAdminTokenHexValidation verifies hexadecimal character rules,
// including uppercase tokens, invalid characters, and internal whitespace.
func TestLoadAdminTokenHexValidation(t *testing.T) {
	// Test cases
	tests := []struct {
		name      string
		token     string
		wantError string
	}{
		{
			name:      "invalid characters",
			token:     strings.Repeat("a", 63) + "G",
			wantError: "ADMIN_API_TOKEN contains invalid hexadecimal characters",
		},
		{
			name:      "valid uppercase characters",
			token:     strings.Repeat("A", 64),
			wantError: "",
		},
		{
			name:      "internal whitespace",
			token:     strings.Repeat("a", 31) + " " + strings.Repeat("a", 32),
			wantError: "ADMIN_API_TOKEN contains invalid hexadecimal characters",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			validDatabaseURL := "postgres://localhost/testdb"
			t.Setenv("DATABASE_URL", validDatabaseURL)

			t.Setenv("ADMIN_API_TOKEN", tc.token)

			cfg, err := Load()
			// Successful validation case
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}

				if cfg.AdminAPIToken != tc.token {
					t.Errorf("Expected token %q, actual token %q", tc.token, cfg.AdminAPIToken)
				}
				return
			}

			// Expected validation failure
			if err == nil {
				t.Fatalf("expected error %q, got nil", tc.wantError)
			}

			if err.Error() != tc.wantError {
				t.Errorf("expected error %q, got %q", tc.wantError, err.Error())
			}
		})
	}
}

// TestLoadWhitespaceNormalization verifies that surrounding whitespace
// is removed and whitespace-only configuration values are rejected.
func TestLoadWhitespaceNormalization(t *testing.T) {
	// Test cases
	tests := []struct {
		name            string
		databaseURL     string
		token           string
		wantDatabaseURL string
		wantToken       string
		wantError       string
	}{
		{
			name:            "database url surrounded by spaces",
			databaseURL:     "  postgres://localhost/testdb  ",
			token:           strings.Repeat("a", 64),
			wantDatabaseURL: "postgres://localhost/testdb",
			wantToken:       strings.Repeat("a", 64),
			wantError:       "",
		},
		{
			name:            "database url contains only spaces",
			databaseURL:     "  ",
			token:           strings.Repeat("a", 64),
			wantDatabaseURL: "",
			wantToken:       "",
			wantError:       "DATABASE_URL environment variable is not set",
		},
		{
			name:            "admin token surrounded by spaces",
			databaseURL:     "postgres://localhost/testdb",
			token:           " " + strings.Repeat("a", 32) + strings.Repeat("a", 32) + " ",
			wantDatabaseURL: "postgres://localhost/testdb",
			wantToken:       strings.Repeat("a", 64),
			wantError:       "",
		},
		{
			name:            "admin token contains only spaces",
			databaseURL:     "postgres://localhost/testdb",
			token:           "  ",
			wantDatabaseURL: "",
			wantToken:       "",
			wantError:       "ADMIN_API_TOKEN must be 64 hexadecimal characters",
		},
		{
			name:            "both values surrounded by tabs and newlines",
			databaseURL:     "\t\npostgres://localhost/testdb\t\n",
			token:           "\t\n" + strings.Repeat("a", 32) + strings.Repeat("a", 32) + "\t\n",
			wantDatabaseURL: "postgres://localhost/testdb",
			wantToken:       strings.Repeat("a", 64),
			wantError:       "",
		},
		{
			name:            "both environment variables empty",
			databaseURL:     "",
			token:           "",
			wantDatabaseURL: "",
			wantToken:       "",
			wantError:       "DATABASE_URL environment variable is not set",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", tc.databaseURL)

			t.Setenv("ADMIN_API_TOKEN", tc.token)

			cfg, err := Load()
			// Verify expected error behavior.
			if tc.wantError != "" && err == nil {
				t.Fatalf("expected error %q, got nil", tc.wantError)
			}
			if err != nil && err.Error() != tc.wantError {
				t.Fatalf("Expected error: %q, got error: %q", tc.wantError, err.Error())
			}

			// Verify normalized values on success and zero values on failure
			if cfg.DatabaseURL != tc.wantDatabaseURL {
				t.Errorf("Expected database URL: %q, got database URL: %q", tc.wantDatabaseURL, cfg.DatabaseURL)
			}
			if cfg.AdminAPIToken != tc.wantToken {
				t.Errorf("Expected token: %q, got token: %q", tc.wantToken, cfg.AdminAPIToken)
			}
		})
	}
}
