# Reverse Proxy

A lightweight yet complete **reverse proxy** written in Go.

This project was created as a strong portfolio piece and interview preparation material for roles such as **Go Backend Developer**, **Platform Engineer**, or **SRE**.

## Features (Iteration 2)

### Core
- Reverse Proxy using Go's `httputil.NewSingleHostReverseProxy`
- **Round Robin Load Balancing** powered by `sync/atomic`
- Health Checks with automatic failover
- IP-based Rate Limiting (token bucket algorithm)

### Observability
- Full **Prometheus metrics** at `/metrics`:
    - `proxy_requests_total{method, path, backend, status}`
    - `proxy_request_duration_seconds` (histogram)
    - `backend_healthy` (gauge 0/1)
    - `rate_limit_rejected_total`
- Request logging middleware

### Operability (Control Plane)
- **Admin API** for real-time manual backend management:
    - `GET /admin/backends` — list backends with alive status
    - `POST /admin/backends/{index}/down`
    - `POST /admin/backends/{index}/up`

### Improved Health Checks
- Configurable health check path (`health_check_path`)
- Uses `HEAD` requests instead of `GET` (lighter on backends)
- Logs **only state changes** (no spam every 5 seconds)
- Automatically updates Prometheus `backend_healthy` metric

### Other
- Basic unit tests in the `balancer` package
- YAML configuration
- Production-ready Dockerfile + docker-compose

## Quick Start

```bash
docker-compose up --build
```

Available endpoints:

| Endpoint                              | Description             |
|---------------------------------------|-------------------------|
| `http://localhost:8080`               | Reverse proxy           |
| `http://localhost:8080/admin/backends`| Admin API               |
| `http://localhost:8080/metrics`       | Prometheus metrics      |

### Examples

```bash
# Normal traffic
curl http://localhost:8080

# Check backend status
curl http://localhost:8080/admin/backends

# Manually disable backend #0
curl -X POST http://localhost:8080/admin/backends/0/down

# Re-enable it
curl -X POST http://localhost:8080/admin/backends/0/up

# View metrics
curl -s http://localhost:8080/metrics | grep -E 'proxy_requests_total|backend_healthy|rate_limit_rejected'
```

## Configuration

`config/config.yaml`:

```yaml
port: 8080

backends:
  - http://localhost:8081
  - http://localhost:8082

health_check_interval: 5s
health_check_path: /health

rate_limit:
  requests_per_minute: 100
```

## Architecture

```
Client
   │
   ▼
Reverse Proxy (8080)
   ├── Middleware: Logger → Rate Limiter → Proxy
   ├── Admin API (/admin/*)
   └── Prometheus (/metrics)
          │
   +------+------+
   │             │
   ▼             ▼
Backend 1     Backend 2
```

- **Load Balancing**: Round Robin, skips unhealthy backends
- **Health Checks**: dedicated goroutine
- **Failover**: automatic and instant

## Run Locally (without Docker)

```bash
# Terminals 1 & 2 - backends
docker run --rm -p 8081:80 nginx:alpine
docker run --rm -p 8082:80 nginx:alpine

# Terminal 3 - proxy
go run ./cmd/server
```

## Development

```bash
# Tests
go test ./internal/balancer -v

# Build
go build -o proxy ./cmd/server

# Docker
docker build -t reverse-proxy .
```

## Project Structure

```
reverse-proxy/
├── cmd/server/main.go
├── internal/
│   ├── admin/              # Admin API (control plane)
│   ├── balancer/           # Round Robin + tests
│   ├── health/             # Health checks + Prometheus
│   ├── metrics/            # Prometheus metrics
│   ├── proxy/              # Reverse proxy + metrics recording
│   ├── ratelimit/          # Rate limiting + metrics
│   └── logger/
├── config/config.yaml
├── docker-compose.yml
├── Dockerfile
├── .gitignore
└── README.md
```

## What This Demonstrates in Interviews

This project is specifically designed to shine in interviews for:

- Go Backend Developer
- Platform Engineer
- SRE / Infrastructure Engineer (with Go)

### Topics you can discuss confidently:

- Reverse Proxy internals (`httputil`)
- Load balancing with atomics
- Concurrency in Go (`sync/atomic`, `RWMutex`, goroutines)
- Middleware pattern
- Rate limiting (token bucket)
- **Observability** — instrumenting Go services with Prometheus
- **Operability** — building a simple control plane via Admin API
- Health checking strategies and failover
- Unit testing in Go
- Clean project structure and separation of concerns

## Roadmap (Suggested Future Work)

- Weighted Round Robin / Least Connections
- Circuit Breaker
- Structured logging (`log/slog`)
- Full integration tests (`httptest`)
- TLS termination
- Connection pooling tuning
- Redis-backed rate limiting
- Authentication for Admin API

## Tech Stack

- Go 1.24+
- `net/http` + `httputil`
- `sync/atomic`
- `golang.org/x/time/rate`
- `github.com/prometheus/client_golang`
- `gopkg.in/yaml.v3`

## Lines of Code

Currently **~500 lines of pure Go** (including tests).

## License

MIT (feel free to change)