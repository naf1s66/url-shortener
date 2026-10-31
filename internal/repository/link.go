package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type URL struct {
	OriginalURL string
	ExpiresAt   sql.NullTime
	RevokedAt   sql.NullTime
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	repo := &Repository{
		pool: pool,
	}
	return repo
}

func (repo *Repository) GetOrCreateURLID(ctx context.Context, originalURL string) (string, error) {
	var urlID string
	// Query
	// ON CONFLICT helps avoid failure when a URL already exists.
	insertQuery := `INSERT INTO urls (url) VALUES ($1) ON CONFLICT (url) DO NOTHING RETURNING id`
	// Using context associated with request
	// This allows DB work to be cancelled with request.
	err := repo.pool.QueryRow(ctx, insertQuery, originalURL).Scan(&urlID)
	if err != nil {
		switch {
		// Checking whether returned error matches no returned row error.
		// This happens when URL already exists so Postgres returns nothing.
		case errors.Is(err, pgx.ErrNoRows):
			// No row is inserted so nothing is returned.
			// Hence we retrieve existing URL's ID.
			retrieveQuery := `SELECT id FROM urls WHERE url = $1`
			err = repo.pool.QueryRow(ctx, retrieveQuery, originalURL).Scan(&urlID)
			if err != nil {
				return "", err
			}
		default:
			return "", err
		}
	}
	return urlID, nil
}

func (repo *Repository) CreateShortURL(ctx context.Context, shortCode string, urlID string, expiresAt *time.Time, revocationTokenHash []byte) error {
	// Every short link has its own code, expiration and revocation-token hash.
	shortCodeInsert := "INSERT INTO short_urls (short_code, url_id, expires_at, revocation_token_hash) VALUES ($1, $2, $3, $4)"
	// Executing insert
	_, err := repo.pool.Exec(ctx, shortCodeInsert, shortCode, urlID, expiresAt, revocationTokenHash)
	if err != nil {
		return err
	}
	return nil
}

func (repo *Repository) FindByShortCode(ctx context.Context, shortCode string) (URL, error) {
	var retrieved URL
	getURLQuery := "SELECT u.url, s.expires_at, s.revoked_at FROM short_urls AS s JOIN urls AS u ON u.id = s.url_id WHERE s.short_code = $1"
	err := repo.pool.QueryRow(ctx, getURLQuery, shortCode).Scan(&retrieved.OriginalURL, &retrieved.ExpiresAt, &retrieved.RevokedAt)
	if err != nil {
		return URL{}, err
	}
	return retrieved, nil
}

func (repo *Repository) AdminRevokeByShortCode(ctx context.Context, shortCode string) error {
	// Atomic and idempotent database update to prevent TOCTOU
	// We use COALESCE so every query returns same result (1 row affected if short code exists)
	// Without changing revoked_at timestamp.
	// We did not use ``AND revoked_at IS NULL`` check because that would not be idempotent and subsequent
	// query runs after initial revocation would return different result (0 rows affected).
	adminRevokeURLQuery := `UPDATE short_urls
	SET revoked_at = COALESCE(revoked_at, NOW())
	WHERE short_code = $1`
	tag, err := repo.pool.Exec(ctx, adminRevokeURLQuery, shortCode)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (repo *Repository) RevokeByShortCode(ctx context.Context, shortCode string, revocationTokenHash []byte) error {
	// One SQL UPDATE statement to match resource and auth hash atomically to
	// prevent TOCTOU (time of check to time of use problem)
	revokeURLQuery := `UPDATE short_urls
	SET revoked_at = COALESCE(revoked_at, NOW())
	WHERE short_code = $1
	AND revocation_token_hash = $2`
	tag, err := repo.pool.Exec(ctx, revokeURLQuery, shortCode, revocationTokenHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
