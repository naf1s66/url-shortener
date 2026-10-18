# Go URL Shortener

A backend engineering learning project built with Go and PostgreSQL.

The goal is to build a production-style URL shortener while learning API design, database engineering, authentication, security, caching, containerization, testing, and observability.

## Tech Stack

- **Language:** Go (`net/http`)
- **Database:** PostgreSQL
- **Database access:** `pgx` / `pgxpool`
- **Cryptography:** Go standard library (`crypto/rand`, `crypto/sha256`, `crypto/subtle`)

**Planned:** Redis, Docker, Docker Compose, k6, Prometheus, Grafana, and GitHub Actions.

## Implemented Features

- HTTP server and health endpoint
- PostgreSQL connection pooling
- JSON request handling and URL validation
- Original URL deduplication using a unique constraint and `ON CONFLICT`
- Cryptographically secure six-character Base62 short-code generation
- Collision detection with bounded retries
- Optional URL expiration using RFC3339 timestamps
- HTTP `302 Found` redirects with `Cache-Control: no-store`
- Expired, revoked, and nonexistent link handling (`410` / `404`)
- Per-link revocation tokens generated with `crypto/rand`
- SHA-256 revocation-token hashes stored in PostgreSQL
- Authenticated self-service URL revocation
- Private administrator token for revoking any existing URL
- Timing-resistant administrator token comparison
- Atomic, idempotent revocation using PostgreSQL and `COALESCE`
- `Cache-Control: no-store` on responses containing sensitive tokens

## API Endpoints

| Method | Endpoint | Description |
| --- | --- | --- |
| `GET` | `/health` | Check server health |
| `POST` | `/shorten` | Create a shortened URL |
| `GET` | `/{shortCode}` | Redirect to the original URL |
| `PATCH` | `/{shortCode}/revoke` | Revoke a URL using its per-link token |
| `PATCH` | `/admin/links/{shortCode}/revoke` | Revoke any URL using the admin token |

### Create a Short URL

**Request:** `POST /shorten`

```json
{
  "url": "https://example.com",
  "expires_at": "2027-12-31T23:59:59+06:00"
}
```

The `expires_at` field is optional. When omitted, the shortened URL has no scheduled expiration.

A successful request returns `201 Created` with a response shaped like:

```json
{
  "received_url": "https://example.com",
  "shortened_url": "http://localhost:8080/Ab3xY9",
  "revocation_token": "<generated-private-token>"
}
```

The revocation token is returned on creation. The server stores its SHA-256 hash, not the original token. The client must retain the original token securely if it wants to revoke the link later.

### Redirect

**Request:** `GET /{shortCode}`

| Link state | Response |
| --- | --- |
| Active | `302 Found`, with a `Location` header |
| Expired | `410 Gone` |
| Revoked | `404 Not Found` |
| Unknown short code | `404 Not Found` |

### Self-Service Revocation

**Request:** `PATCH /{shortCode}/revoke`

```http
Authorization: Bearer <revocation-token>
```

The server decodes the Base64URL token, computes its SHA-256 hash, and conditionally updates the matching short code and hash in PostgreSQL.

- `204 No Content`: Link revoked, including a repeat request with the correct token.
- `401 Unauthorized`: Missing, malformed, or incorrectly formatted bearer credential.
- `404 Not Found`: No matching short code and token hash.

### Administrator Revocation

**Request:** `PATCH /admin/links/{shortCode}/revoke`

```http
Authorization: Bearer <admin-token>
```

The server checks the supplied credential against `ADMIN_API_TOKEN` using a timing-resistant comparison. An authenticated administrator can revoke any existing link without its per-link token.

- `204 No Content`: Link revoked, including a repeat authorized request.
- `401 Unauthorized`: Missing or incorrect admin credential.
- `404 Not Found`: Short code does not exist.

Revocation preserves the original `revoked_at` timestamp through `COALESCE(revoked_at, NOW())`.

## Local Development

### Requirements

- Go
- PostgreSQL
- OpenSSL (for generating the administrator token)
- Bash (for the startup command below)

### Setup

1. Create a PostgreSQL database named `url_shortener` and a database user with appropriate access.
2. Apply the SQL migrations in `migrations/` in numerical order (`001`, `002`, `003`). Migrations are currently run manually.
3. Copy the environment template:

   ```bash
   cp .env.example .env
   ```

4. Edit `.env` and set `DATABASE_URL` to your local PostgreSQL connection string.
5. Generate an administrator secret:

   ```bash
   openssl rand -hex 32
   ```

6. Add the generated **64-character hexadecimal value** to `ADMIN_API_TOKEN` in `.env`. Do not use the example placeholder as a real token.

   Example configuration with placeholders:

   ```dotenv
   DATABASE_URL='postgres://urlshortener:YOUR_PASSWORD@127.0.0.1:5432/url_shortener?sslmode=disable'
   ADMIN_API_TOKEN='REPLACE_WITH_64_HEXADECIMAL_CHARACTERS'
   ```

7. Run the API from the project directory:

   ```bash
   (set -a; source .env; set +a; go run .)
   ```

The API listens on `http://localhost:8080`.

Check the health endpoint:

```bash
curl http://localhost:8080/health
```

Expected response:

```json
{"status":"ok"}
```

> **Security:** Never commit `.env` or share actual administrator or per-link tokens. The localhost HTTP setup is for local development; use HTTPS when deploying the API publicly.

### Current Testing

The endpoint flows have been manually checked with `curl`, including creation, redirect, self-service revocation, administrator revocation, repeated revocation, invalid credentials, and unknown links. Automated unit and integration tests are planned.

Compile/check the Go code with:

```bash
gofmt -w main.go
go test ./...
go vet ./...
```

## Roadmap

- Developer commands with a `Makefile`
- OpenAPI specification and Swagger UI
- Configuration and layered-architecture refactoring
- Automated unit and integration tests
- Redis caching and invalidation
- Docker and Docker Compose
- Load testing with k6
- Structured logging, Prometheus metrics, and Grafana dashboards
- CI/CD with GitHub Actions
- Deployment

## Project Goal

Learn backend engineering by designing, implementing, testing, and understanding each component from scratch, including the tradeoffs involved in building reliable and secure systems.
