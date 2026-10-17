# Go URL Shortener

A backend engineering learning project using Go and PostgreSQL.

The goal is to build a production-style URL shortener while learning API design, database engineering, caching, containerization, testing, and observability.

## Tech Stack

- Go (`net/http`)
- PostgreSQL
- pgx / pgxpool

Planned: Redis, Docker, k6, Prometheus, Grafana, GitHub Actions.

## Implemented

- HTTP server and health endpoint
- URL validation
- PostgreSQL connection pooling
- Original URL deduplication
- Secure Base62 short-code generation
- Short-code persistence
- Unique constraint detection and bounded collision retries
- HTTP 201 Created response
- Optional URL expiration with RFC3339 timestamps
- Validation of expiration dates before persistence
- HTTP 302 redirects with Cache-Control: no-store
- Expired and revoked link handling (404/410 responses)
- Secure per-link revocation token generation using crypto/rand
- SHA-256 revocation token hashes stored in PostgreSQL
- Sensitive token responses marked Cache-Control: no-store

## API

| Method | Endpoint | Status |
|---|---|---|
| GET | `/health` | Implemented |
| POST | `/shorten` | Implemented |
| GET | `/{shortCode}` | Implemented |
| PATCH | `/{shortCode}/revoke` | Planned |
| PATCH | `/admin/links/{shortCode}/revoke` | Planned |

## Local Development

Requires Go and PostgreSQL.

1. Create a PostgreSQL database named `url_shortener`.
2. Apply the SQL migrations in `migrations/` in numerical order.
3. Copy the example environment file:

   ```bash
   cp .env.example .env
   ```
4. Edit `.env` and set your PostgreSQL connection details.
5. Start the API:

   ```bash
   (set -a; source .env; set +a; go run .)
   ```

The server runs at `http://localhost:8080`.

Check the health endpoint:

```bash
curl http://localhost:8080/health
```

Note: Migrations are currently applied manually and are not automatically version-tracked.

## Roadmap

- Token-authenticated self-service revocation
- Private administrator revocation and moderation
- OpenAPI specification and Swagger UI
- Automated testing and layered architecture
- Redis caching and invalidation
- Docker and Docker Compose
- Load testing using k6
- Prometheus and Grafana observability
- CI/CD and deployment

## Project Goal

Learn backend engineering by implementing and understanding each component from scratch.
