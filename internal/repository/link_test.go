//go:build integration

package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestGetOrCreateURLID(t *testing.T) {
	repo, pool := setupTestRepository(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	cleanupQuery := `DELETE FROM urls WHERE url = $1`

	testURL := fmt.Sprintf(
		"https://example.com/integration-test-%d",
		time.Now().UnixNano(),
	)

	firstID, err := repo.GetOrCreateURLID(ctx, testURL)
	if err != nil {
		t.Fatalf("Failed to get or create URL ID: %v", err)
	}

	if firstID == "" {
		t.Fatalf("Expected a non-empty URL ID, got empty string")
	}

	registerDBCleanup(t, pool, cleanupQuery, testURL)

	secondID, err := repo.GetOrCreateURLID(ctx, testURL)
	if err != nil {
		t.Fatalf("Failed to get or create URL ID: %v", err)
	}

	if secondID == "" {
		t.Fatalf("Expected a non-empty URL ID, got empty string")
	}

	if firstID != secondID {
		t.Errorf(
			"Expected identical URL IDs, got %q and %q",
			firstID,
			secondID,
		)
	}

	checkQuery := `SELECT COUNT(*) FROM urls WHERE url = $1`

	var count int

	err = pool.QueryRow(ctx, checkQuery, testURL).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count URLs: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 URL entry, got %d", count)
	}
}

func TestCreateShortURL(t *testing.T) {
	repo, pool := setupTestRepository(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	cleanupShortURLQuery := `DELETE FROM short_urls WHERE short_code = $1`

	_, ID := createTestURL(t, ctx, repo, pool)

	shortCode := "Tst001"

	expiresAt := time.Now().
		UTC().
		Add(time.Hour).
		Truncate(time.Microsecond)

	hash := sha256.Sum256([]byte("integration-test-token"))

	err := repo.CreateShortURL(
		ctx,
		shortCode,
		ID,
		&expiresAt,
		hash[:],
	)
	if err != nil {
		t.Fatalf("Failed to create short URL: %v", err)
	}

	registerDBCleanup(t, pool, cleanupShortURLQuery, shortCode)

	var storedURLID string
	var storedExpiresAt time.Time
	var storedHash []byte

	checkQuery := `
		SELECT url_id, expires_at, revocation_token_hash
		FROM short_urls
		WHERE short_code = $1
	`

	err = pool.QueryRow(ctx, checkQuery, shortCode).
		Scan(&storedURLID, &storedExpiresAt, &storedHash)

	if err != nil {
		t.Fatalf("Failed to check short URL: %v", err)
	}

	if storedURLID != ID {
		t.Errorf("Expected URL ID %q, got %q", ID, storedURLID)
	}

	if !storedExpiresAt.Equal(expiresAt) {
		t.Errorf(
			"Expected expires at %v, got %v",
			expiresAt,
			storedExpiresAt,
		)
	}

	if !bytes.Equal(storedHash, hash[:]) {
		t.Errorf("Expected hash %x, got %x", hash[:], storedHash)
	}
}

func TestCreateShortURL_DuplicateCode(t *testing.T) {
	repo, pool := setupTestRepository(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	cleanupShortURLQuery := `DELETE FROM short_urls WHERE short_code = $1`

	_, ID := createTestURL(t, ctx, repo, pool)

	shortCode := "Tst002"

	expiresAt := time.Now().
		UTC().
		Add(time.Hour).
		Truncate(time.Microsecond)

	hash := sha256.Sum256([]byte("integration-test-token"))

	err := repo.CreateShortURL(
		ctx,
		shortCode,
		ID,
		&expiresAt,
		hash[:],
	)
	if err != nil {
		t.Fatalf("Failed to create short URL: %v", err)
	}

	registerDBCleanup(t, pool, cleanupShortURLQuery, shortCode)
	err = repo.CreateShortURL(
		ctx,
		shortCode,
		ID,
		&expiresAt,
		hash[:],
	)
	if err == nil {
		t.Fatalf("Expected a duplicate short-code error, but insertion succeeded")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("Expected PostgreSQL error, got %v", err)
	}

	if pgErr.Code != "23505" {
		t.Errorf("Expected SQLSTATE 23505, got %s", pgErr.Code)
	}

	if pgErr.ConstraintName != "short_urls_short_code_key" {
		t.Errorf(
			"Expected constraint short_urls_short_code_key, got %q",
			pgErr.ConstraintName,
		)
	}

}

func TestCreateShortURL_InvalidURLID(t *testing.T) {
	repo, _ := setupTestRepository(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	ID := "00000000-0000-0000-0000-000000000000" //Non existent URL ID

	shortCode := "Tst003"

	expiresAt := time.Now().
		UTC().
		Add(time.Hour).
		Truncate(time.Microsecond)

	hash := sha256.Sum256([]byte("integration-test-token"))

	err := repo.CreateShortURL(
		ctx,
		shortCode,
		ID,
		&expiresAt,
		hash[:],
	)
	if err == nil {
		t.Fatalf("Expected insertion to fail due to non existed URL ID, but insertion succeeded")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("Expected PostgreSQL error, got %v", err)
	}

	if pgErr.Code != "23503" {
		t.Errorf("Expected SQLSTATE 23503, got %s", pgErr.Code)
	}

	if pgErr.ConstraintName != "short_urls_url_id_fkey" {
		t.Errorf(
			"Expected constraint short_urls_url_id_fkey, got %q",
			pgErr.ConstraintName,
		)
	}

}

func TestFindByShortCode(t *testing.T) {
	repo, pool := setupTestRepository(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	testURL, ID := createTestURL(t, ctx, repo, pool)

	shortCode, _ := createTestShortURL(t, ctx, repo, pool, ID, nil)

	retrieved, err := repo.FindByShortCode(ctx, shortCode)

	if err != nil {
		t.Fatalf("Failed to find short code: %v", err)
	}

	if retrieved.OriginalURL != testURL {
		t.Errorf("Expected original URL %q, got %q", testURL, retrieved.OriginalURL)
	}

	if retrieved.ExpiresAt.Valid {
		t.Errorf("Expected NULL expiration, got %v", retrieved.ExpiresAt.Time)
	}

	if retrieved.RevokedAt.Valid {
		t.Errorf("Expected NULL revocation, got %v", retrieved.RevokedAt.Time)
	}
}

func TestFindByShortCode_WithExpiration(t *testing.T) {
	repo, pool := setupTestRepository(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	testURL, ID := createTestURL(t, ctx, repo, pool)

	expiresAt := time.Now().
		UTC().
		Add(time.Hour).
		Truncate(time.Microsecond)

	shortCode, _ := createTestShortURL(t, ctx, repo, pool, ID, &expiresAt)

	retrieved, err := repo.FindByShortCode(ctx, shortCode)

	if err != nil {
		t.Fatalf("Failed to find short code: %v", err)
	}

	if retrieved.OriginalURL != testURL {
		t.Errorf("Expected original URL %q, got %q", testURL, retrieved.OriginalURL)
	}

	if !retrieved.ExpiresAt.Valid {
		t.Fatal("Expected a non-NULL expiration timestamp")
	}

	if !retrieved.ExpiresAt.Time.Equal(expiresAt) {
		t.Errorf(
			"Expected expiration %v, got %v",
			expiresAt,
			retrieved.ExpiresAt.Time,
		)
	}

	if retrieved.RevokedAt.Valid {
		t.Errorf("Expected NULL revocation, got %v", retrieved.RevokedAt.Time)
	}
}

func TestFindByShortCode_WithRevocation(t *testing.T) {
	repo, pool := setupTestRepository(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	testURL, ID := createTestURL(t, ctx, repo, pool)

	shortCode, _ := createTestShortURL(t, ctx, repo, pool, ID, nil)

	revokedAt := time.Now().
		UTC().
		Truncate(time.Microsecond)

	revokeQuery := `UPDATE short_urls
	SET revoked_at = $1
	WHERE short_code = $2;`

	commandTag, err := pool.Exec(ctx, revokeQuery, revokedAt, shortCode)

	if err != nil {
		t.Fatalf("Failed to update revocation timestamp: %v", err)
	}

	if commandTag.RowsAffected() != 1 {
		t.Fatalf("Expected 1 row to be affected, but affected %d rows", commandTag.RowsAffected())
	}

	retrieved, err := repo.FindByShortCode(ctx, shortCode)

	if err != nil {
		t.Fatalf("Failed to find short code: %v", err)
	}

	if retrieved.OriginalURL != testURL {
		t.Errorf("Expected original URL %q, got %q", testURL, retrieved.OriginalURL)
	}

	if retrieved.ExpiresAt.Valid {
		t.Errorf("Expected NULL expiration timestamp, got %v", retrieved.ExpiresAt.Time)
	}

	if !retrieved.RevokedAt.Valid {
		t.Fatal("Expected non-NULL revocation timestamp")
	}

	if !retrieved.RevokedAt.Time.Equal(revokedAt) {
		t.Errorf(
			"Expected revocation %v, got %v",
			revokedAt,
			retrieved.RevokedAt.Time,
		)
	}
}

func TestFindByShortCode_NotFound(t *testing.T) {
	repo, _ := setupTestRepository(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	shortCode := "NA0001"
	_, err := repo.FindByShortCode(ctx, shortCode)

	if err == nil {
		t.Fatal("Expected pgx.ErrNoRows for nonexistent short code, got nil")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("Expected pgx.ErrNoRows, got %v", err)
	}
}
