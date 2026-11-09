//go:build integration

package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupTestRepository(t *testing.T) (*Repository, *pgxpool.Pool) {
	t.Helper()

	testDatabaseURL := os.Getenv("TEST_DATABASE_URL")
	if testDatabaseURL == "" {
		t.Fatalf("Need a valid test DB URL")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	pool, err := pgxpool.New(ctx, testDatabaseURL)
	if err != nil {
		t.Fatalf("Unable to create connection pool: %v", err)
	}

	t.Cleanup(pool.Close)

	checkDBQuery := `SELECT current_database()`

	var dbName string

	err = pool.QueryRow(ctx, checkDBQuery).Scan(&dbName)
	if err != nil {
		t.Fatalf("Failed to verify test database: %v", err)
	}

	if dbName != "url_shortener_test" {
		t.Fatalf("Refusing to run integration tests against database %q", dbName)
	}

	repo := New(pool)
	return repo, pool
}

func registerDBCleanup(
	t *testing.T,
	pool *pgxpool.Pool,
	query string,
	args ...any,
) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancel()

		_, err := pool.Exec(ctx, query, args...)
		if err != nil {
			t.Errorf("Database cleanup failed: %v", err)
		}
	})
}

func createTestURL(
	t *testing.T,
	ctx context.Context,
	repo *Repository,
	pool *pgxpool.Pool,
) (string, string) {
	t.Helper()

	cleanupURLQuery := `DELETE FROM urls WHERE id = $1`

	testURL := fmt.Sprintf(
		"https://example.com/integration-test-%d",
		time.Now().UnixNano(),
	)

	ID, err := repo.GetOrCreateURLID(ctx, testURL)
	if err != nil {
		t.Fatalf("Failed to get or create URL ID: %v", err)
	}

	if ID == "" {
		t.Fatalf("Expected a non-empty URL ID, got empty string")
	}

	registerDBCleanup(t, pool, cleanupURLQuery, ID)

	return testURL, ID
}

func createTestShortURL(
	t *testing.T,
	ctx context.Context,
	repo *Repository,
	pool *pgxpool.Pool,
	urlID string,
	expiresAt *time.Time,
) (string, []byte) {
	t.Helper()

	var randomBytes [3]byte

	_, err := rand.Read(randomBytes[:])
	if err != nil {
		t.Fatalf("Failed to generate test short code: %v", err)
	}

	generatedShortCode := fmt.Sprintf("%x", randomBytes)
	t.Logf("Generated short code: %s", generatedShortCode)

	hash := sha256.Sum256([]byte("integration-test-token"))

	err = repo.CreateShortURL(
		ctx,
		generatedShortCode,
		urlID,
		expiresAt,
		hash[:],
	)
	if err != nil {
		t.Fatalf("Failed to create short URL: %v", err)
	}

	cleanupShortURLQuery := `DELETE FROM short_urls WHERE short_code = $1`

	registerDBCleanup(
		t,
		pool,
		cleanupShortURLQuery,
		generatedShortCode,
	)

	return generatedShortCode, hash[:]
}
