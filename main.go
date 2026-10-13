package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}
	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal("Unable to create connection pool:", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatal("Database ping failed:", err)
	}
	log.Println("Database connected successfully")
	http.HandleFunc("GET /health", healthHandler)
	http.HandleFunc("POST /shorten", func(w http.ResponseWriter, r *http.Request) {
		shortenHandler(w, r, pool)
	})
	http.HandleFunc("GET /{shortCode}", func(w http.ResponseWriter, r *http.Request) {
		redirectHandler(w, r, pool)
	})
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}

type ShortenRequest struct {
	URL string `json:"url"`
}

func shortenHandler(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) {
	var req ShortenRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	trimmedURL := strings.TrimSpace(req.URL)
	if trimmedURL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}
	if len(trimmedURL) > 2048 {
		http.Error(w, "URL exceeds maximum length of 2048 bytes", http.StatusBadRequest)
		return
	}
	parsedURL, err := url.Parse(trimmedURL)
	if err != nil {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}
	if parsedURL.Hostname() == "" || parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}
	var urlID string
	insertQuery := `INSERT INTO urls (url) VALUES ($1) ON CONFLICT (url) DO NOTHING RETURNING id`
	err = pool.QueryRow(r.Context(), insertQuery, trimmedURL).Scan(&urlID)
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			retrieveQuery := `SELECT id FROM urls WHERE url = $1`
			err = pool.QueryRow(r.Context(), retrieveQuery, trimmedURL).Scan(&urlID)
			if err != nil {
				log.Printf("Failed to retrieve existing URL ID: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
		default:
			log.Printf("Failed to save URL: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
	log.Println("Resolved URL ID:", urlID)
	var generatedShortCode string
	savedSuccessfully := false

	for i := 0; i < 5; i++ {
		var pgErr *pgconn.PgError

		generatedShortCode, err = generateShortCode()
		if err != nil {
			log.Printf("Failed to generate short code: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		log.Println("Generated short code:", generatedShortCode)
		shortCodeInsert := "INSERT INTO short_urls (short_code, url_id) VALUES ($1, $2)"
		_, err = pool.Exec(r.Context(), shortCodeInsert, generatedShortCode, urlID)
		if err == nil {
			log.Println("Short code saved successfully:", generatedShortCode)
			savedSuccessfully = true
			break
		}
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "short_urls_short_code_key" {
			log.Println("Short code collision detected, retrying...")
			continue
		}
		log.Printf("Failed to save short code: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if !savedSuccessfully {
		log.Println("Failed to generate a unique short code after 5 attempts")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	shortenedURL := "http://localhost:8080/" + generatedShortCode
	response := map[string]string{"received_url": trimmedURL, "shortened_url": shortenedURL}
	httpStatus := http.StatusCreated
	w.WriteHeader(httpStatus)
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(response); err != nil {
		log.Println("Error encoding response:", err)
	}
}

func redirectHandler(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) {
	shortCode := r.PathValue("shortCode")
	var originalURL string
	var expiresAt sql.NullTime
	var revokedAt sql.NullTime
	w.Header().Set("Cache-Control", "no-store")
	if shortCode == "" {
		http.Error(w, "Short code is required", http.StatusBadRequest)
		return
	}
	getURLQuery := "SELECT u.url, s.expires_at, s.revoked_at FROM short_urls AS s JOIN urls AS u ON u.id = s.url_id WHERE s.short_code = $1"
	err := pool.QueryRow(r.Context(), getURLQuery, shortCode).Scan(&originalURL, &expiresAt, &revokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Short code not found", http.StatusNotFound)
			return
		}
		log.Printf("Failed to retrieve URL for short code %s: %v", shortCode, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	log.Printf(
		"Resolved %s -> %s | Expires: %v | Revoked: %v",
		shortCode,
		originalURL,
		expiresAt,
		revokedAt,
	)
	if revokedAt.Valid {
		http.Error(w, "Short code has been revoked", http.StatusNotFound)
		return
	}
	if expiresAt.Valid && !time.Now().Before(expiresAt.Time) {
		http.Error(w, "Short code has expired", http.StatusGone)
		return
	}
	http.Redirect(w, r, originalURL, http.StatusFound)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response := map[string]string{"status": "ok"}
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(response); err != nil {
		log.Println("Error encoding response:", err)
	}
}

func generateShortCode() (string, error) {
	const codeLength = 6
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var builder strings.Builder
	builder.Grow(codeLength)
	alphabetSize := big.NewInt(int64(len(alphabet)))
	for i := 0; i < codeLength; i++ {
		n, err := rand.Int(
			rand.Reader,
			alphabetSize,
		)
		if err != nil {
			return "", err
		}
		index := n.Int64()
		char := alphabet[index]
		builder.WriteByte(char)
	}
	result := builder.String()
	return result, nil
}
