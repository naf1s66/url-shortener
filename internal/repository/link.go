package repository

import (
	"context"
	"database/sql"

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
