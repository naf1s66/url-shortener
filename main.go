package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/naf1s66/url-shortener/internal/config"
	"github.com/naf1s66/url-shortener/internal/repository"
)

// main loads config, connects to PostgreSQL, and registers HTTP routes.
func main() {
	// Load and validate application configuration before starting services.
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	// Limiting databse startup operations to five seconds.
	// context helps carry cancellation and deadline info across
	// function calls.
	// Created context here that allows
	// operations to run for at most 5 seconds before signalling
	// that it should stop. We can pass this to other operations
	// such as database queries, HTTP requests.
	// But we are only using it for database startup operations in this app.
	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	// Executes when main returns
	defer cancel()
	// The connection pool reuses database connections across HTTP requests
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Unable to create connection pool:", err)
	}
	defer pool.Close()
	// Pinging DB once.
	if err := pool.Ping(ctx); err != nil {
		log.Fatal("Database ping failed:", err)
	}
	log.Println("Database connected successfully")
	// Initializing repository instance
	repo := repository.New(pool)
	// Routes
	http.HandleFunc("GET /health", healthHandler)
	http.HandleFunc("POST /shorten", func(w http.ResponseWriter, r *http.Request) {
		shortenHandler(w, r, pool)
	})
	http.HandleFunc("GET /{shortCode}", func(w http.ResponseWriter, r *http.Request) {
		redirectHandler(w, r, repo)
	})
	http.HandleFunc("PATCH /{shortCode}/revoke", func(w http.ResponseWriter, r *http.Request) {
		revokeHandler(w, r, repo)
	})
	http.HandleFunc("PATCH /admin/links/{shortCode}/revoke", func(w http.ResponseWriter, r *http.Request) {
		adminRevokeHandler(w, r, repo, cfg.AdminAPIToken)
	})
	// Serve the OpenAPI specification for documentation tools.
	http.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		http.ServeFile(w, r, "docs/openapi.yaml")
	})
	// Serve the Swagger UI documentation page.
	http.HandleFunc("GET /docs/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "docs/swagger.html")
	})
	// Server should always start after route registration because ListenAndServe()
	//  is a blocking function
	// "nil" means we are using default router
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}

// This struct maps the incoming JSON body to Go fields
type ShortenRequest struct {
	URL       string     `json:"url"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func shortenHandler(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) {
	var req ShortenRequest
	// Decoding JSON body into ShortenRequest struct.
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	// Removing leading and trailing whitespace.
	trimmedURL := strings.TrimSpace(req.URL)
	if req.ExpiresAt != nil {
		// Checking if expiry provided is after current time.
		if !req.ExpiresAt.After(time.Now()) {
			http.Error(w, "Expiry time cannot be in the past or right now", http.StatusBadRequest)
			return
		}
	}
	if trimmedURL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}
	// Rejecting URLs longer than 2048 bytes.
	if len(trimmedURL) > 2048 {
		http.Error(w, "URL exceeds maximum length of 2048 bytes", http.StatusBadRequest)
		return
	}
	parsedURL, err := url.Parse(trimmedURL)
	if err != nil {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}
	// Ensuring URL provided is an HTTP(S) URL with a hostname.
	if parsedURL.Hostname() == "" || parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}
	var urlID string
	// Query
	// ON CONFLICT helps avoid failure when a URL already exists.
	insertQuery := `INSERT INTO urls (url) VALUES ($1) ON CONFLICT (url) DO NOTHING RETURNING id`
	// Using context associated with request
	// This allows DB work to be cancelled with request.
	err = pool.QueryRow(r.Context(), insertQuery, trimmedURL).Scan(&urlID)
	if err != nil {
		switch {
		// Checking whether returned error matches no returned row error.
		// This happens when URL already exists so Postgres returns nothing.
		case errors.Is(err, pgx.ErrNoRows):
			// No row is inserted so nothing is returned.
			// Hence we retrieve existing URL's ID.
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

	generatedRevocationToken, generatedHash, err := generateRevocationToken()
	if err != nil {
		log.Printf("Failed to generate revocation token: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var generatedShortCode string
	// Variable to track successful insertion
	savedSuccessfully := false

	// 5 bounded retries.
	for i := 0; i < 5; i++ {
		var pgErr *pgconn.PgError

		generatedShortCode, err = generateShortCode()
		if err != nil {
			log.Printf("Failed to generate short code: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		log.Println("Generated short code:", generatedShortCode)
		// Every short link has its own code, expiration and revocation-token hash.
		shortCodeInsert := "INSERT INTO short_urls (short_code, url_id, expires_at, revocation_token_hash) VALUES ($1, $2, $3, $4)"
		// Executing insert
		_, err = pool.Exec(r.Context(), shortCodeInsert, generatedShortCode, urlID, req.ExpiresAt, generatedHash)
		if err == nil {
			log.Println("Short code saved successfully:", generatedShortCode)
			savedSuccessfully = true
			break
		}
		// SQLSTATE 23505 refers to a unique constraint violation.
		// If error is unique violation on short_urls_short_code_key, we retry.
		// Retry only if collision is specifically on short_code column
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "short_urls_short_code_key" {
			log.Println("Short code collision detected, retrying...")
			continue
		}
		log.Printf("Failed to save short code: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// If all 5 attempts collide
	if !savedSuccessfully {
		log.Println("Failed to generate a unique short code after 5 attempts")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// Returning 201 Created with short URL and its one-time revocation secret
	w.Header().Set("Content-Type", "application/json")
	shortenedURL := "http://localhost:8080/" + generatedShortCode
	// Preventing caching because response contains a private auth token.
	w.Header().Set("Cache-Control", "no-store")
	response := map[string]string{
		"received_url":     trimmedURL,
		"shortened_url":    shortenedURL,
		"revocation_token": generatedRevocationToken,
	}
	httpStatus := http.StatusCreated
	w.WriteHeader(httpStatus)
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(response); err != nil {
		log.Println("Error encoding response:", err)
	}
}

func redirectHandler(w http.ResponseWriter, r *http.Request, repo *repository.Repository) {
	shortCode := r.PathValue("shortCode")
	// Checking if link valid
	w.Header().Set("Cache-Control", "no-store")
	if shortCode == "" {
		http.Error(w, "Short code is required", http.StatusBadRequest)
		return
	}
	link, err := repo.FindByShortCode(r.Context(), shortCode)
	// DB layer error returned by repo handled here
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
		link.OriginalURL,
		link.ExpiresAt,
		link.RevokedAt,
	)
	// Checking if revoked_at is non-NULL.
	if link.RevokedAt.Valid {
		http.Error(w, "Short code has been revoked", http.StatusNotFound)
		return
	}
	// Checking if expires_at is non-NULL and if current time not before expiration
	if link.ExpiresAt.Valid && !time.Now().Before(link.ExpiresAt.Time) {
		http.Error(w, "Short code has expired", http.StatusGone)
		return
	}
	http.Redirect(w, r, link.OriginalURL, http.StatusFound)
}

func revokeHandler(w http.ResponseWriter, r *http.Request, repo *repository.Repository) {
	w.Header().Set("Cache-Control", "no-store")
	// Extracting short code from url
	shortCode := r.PathValue("shortCode")
	if shortCode == "" {
		http.Error(w, "Short code is required", http.StatusBadRequest)
		return
	}
	//Reading auth header
	authHeader := r.Header.Get("Authorization")
	//Checking if prefix is Bearer and removing it and extracting string token
	token, found := strings.CutPrefix(authHeader, "Bearer ")
	//Returning 401 Unauthorized if token is missing, empty or malformed
	// strings.TrimSpace helps reject whitespace only tokens too
	if strings.TrimSpace(token) == "" || !found {
		//Notifying client which auth scheme this endpoint expects
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "Authentication Token is required", http.StatusUnauthorized)
		return
	}
	//Decoding to get original 32 bytes
	decodedBytes, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		// Logging for internal analytics while hiding internal details from client
		w.Header().Set("WWW-Authenticate", "Bearer")
		log.Printf("Failed to decode token: %v", err)
		http.Error(w, "Invalid authentication token", http.StatusUnauthorized)
		return
	}
	//Checking if exactly 32 bytes
	if len(decodedBytes) != 32 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "Invalid authentication token", http.StatusUnauthorized)
		return
	}
	//Rehashing
	computedHash := sha256.Sum256(decodedBytes)
	err = repo.RevokeByShortCode(r.Context(), shortCode, computedHash[:])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Ensuring not broadcasting if short code does not exist or hash does not match
			http.Error(w, "Resource not found", http.StatusNotFound)
			return
		}
		log.Printf("Failed to revoke URL for short code %s: %v", shortCode, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func adminRevokeHandler(w http.ResponseWriter, r *http.Request, repo *repository.Repository, adminToken string) {
	w.Header().Set("Cache-Control", "no-store")
	shortCode := r.PathValue("shortCode")
	if shortCode == "" {
		http.Error(w, "Short code is required", http.StatusBadRequest)
		return
	}
	//Reading auth header
	authHeader := r.Header.Get("Authorization")
	//Checking if prefix is Bearer and removing it and extracting string token
	token, found := strings.CutPrefix(authHeader, "Bearer ")
	//Returning 401 Unauthorized if token is missing, empty or malformed
	// strings.TrimSpace helps reject whitespace only tokens too
	if strings.TrimSpace(token) == "" || !found {
		//Notifying client which auth scheme this endpoint expects
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "Authentication Token is required", http.StatusUnauthorized)
		return
	}
	match := subtle.ConstantTimeCompare([]byte(token), []byte(adminToken))
	if match == 0 {
		//Notifying client which auth scheme this endpoint expects
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "Authentication Token is invalid", http.StatusUnauthorized)
		return
	}
	// If we reach here, verification of token is done
	// And we move to db logic
	err := repo.AdminRevokeByShortCode(r.Context(), shortCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Ensuring not broadcasting if short code does not exist
			http.Error(w, "Resource not found", http.StatusNotFound)
			return
		}
		log.Printf("Failed to revoke URL for short code %s: %v", shortCode, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response := map[string]string{"status": "ok"}
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(response); err != nil {
		log.Println("Error encoding response:", err)
	}
}

// This function creates a six-character alphanumeric code using secure randomness.
func generateShortCode() (string, error) {
	const codeLength = 6
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	// Used to incrementally build the short code string.
	var builder strings.Builder
	// Byte allocation does initially
	// Ensuring enough capacity to append {codeLength} additional bytes.
	// Also avoiding unnecessary buffer growth.
	builder.Grow(codeLength)
	alphabetSize := big.NewInt(int64(len(alphabet)))
	// Loop does 6 iterations.
	for i := 0; i < codeLength; i++ {
		// Generates uniformly random index.
		// Output is a pointer to a bigInt.
		n, err := rand.Int(
			rand.Reader,
			// Upper bound
			alphabetSize,
		)
		if err != nil {
			// Have to return string so returning empty string on error.
			return "", err
		}
		// Retrieving interger value.
		index := n.Int64()
		// Selecting character.
		char := alphabet[index]
		// Adding character to string.
		builder.WriteByte(char)
	}
	// Final string.
	result := builder.String()
	return result, nil
}

func generateRevocationToken() (string, []byte, error) {
	// making slice of 32 bytes, all 0 initially
	tokenBytes := make([]byte, 32)
	// Creating 32 bytes
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return "", nil, err
	}
	// Bytes to String Token
	tokenString := base64.RawURLEncoding.EncodeToString(tokenBytes)
	// Hashed Token
	hash := sha256.Sum256(tokenBytes)
	//hash is array of bytes
	// [32]byte -> array, []byte -> slice, hash[:] -> slice of hash array
	return tokenString, hash[:], nil
}
