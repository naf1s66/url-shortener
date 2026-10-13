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

## API

| Method | Endpoint | Status |
|---|---|---|
| GET | `/health` | Implemented |
| POST | `/shorten` | Implemented |
| GET | `/{shortCode}` | Planned |
| PATCH | `/{shortCode}/revoke` | Planned |

## Local Development

Requires Go and PostgreSQL.

1. Create a PostgreSQL database named `url_shortener`.
2. Apply the SQL migrations in `migrations/`.
3. Set the `DATABASE_URL` environment variable.
4. Run:

    go run .

The server runs at `http://localhost:8080`.

## Roadmap

- Redirects, expiration, and revocation
- Automated testing and layered architecture
- Redis caching and invalidation
- Docker and Docker Compose
- Load testing using k6
- Prometheus and Grafana observability
- CI/CD and deployment

## Project Goal

Learn backend engineering by implementing and understanding each component from scratch.
