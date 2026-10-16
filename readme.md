# Reverse Proxy

A lightweight, production-oriented **reverse proxy** written in Go, built as a portfolio project for roles such as **Go Backend Developer**, **Platform Engineer**, or **SRE**.

## Features

### Core Reliability
- Reverse proxying with `net/http/httputil` and header-safe request rewriting
- Three load-balancing strategies: **Round Robin**, **Weighted Round Robin**, **Least Connections**
- Active health checks with configurable thresholds and instant failover
- Graceful shutdown with in-flight request draining
- Configurable timeouts for the server, upstream dial, response headers, and idle connections
- Strict configuration validation with actionable error messages
- Thread-safe backend state (lock-free atomics) and per-backend connection tracking

### Resilience
- Per-backend **circuit breaker** (closed → open → half-open) with fast-fail behaviour
- Configurable **retry policy** with safe defaults (idempotent, bodyless requests only)
- Structured error envelopes with request IDs on gateway failures (`502` / `503` / `504`)

### Security
- **Admin API authentication** via bearer token (constant-time comparison), sourced from environment variables; admin API is disabled when no token is configured (fail-closed)
- Secure client IP detection behind **trusted proxies** (`X-Forwarded-For` with spoofing protection)
- Hop-by-hop header stripping, backend URL validation (no credentials/query/fragment), security headers on admin responses
- Secrets only through environment variables (`ADMIN_TOKEN`, `REDIS_PASSWORD`, `GRAFANA_ADMIN_PASSWORD`)
- Hardened container image: multi-stage build, non-root user, read-only filesystem, dropped capabilities

### Observability
- Structured logging with `log/slog` (JSON or text), request logging with client IP and request ID
- Prometheus metrics: request counts/duration, per-backend errors and timeouts, health status, circuit-breaker state, rate-limit counters, in-flight requests, retries
- Optional **Prometheus + Grafana** stack with a pre-built dashboard for traffic, latency, errors, and backend health

### Traffic Management
- IP-based rate limiting (token bucket) with bounded memory (idle-entry eviction)
- Optional **Redis-backed distributed rate limiting** (atomic Lua token bucket, fail-open on Redis errors)
- Load-balancing strategy selection via configuration

### Performance & Operations
- Upstream connection pooling (idle connection limits, keep-alive tuning, HTTP/2 for TLS upstreams)
- HTTPS termination (TLS 1.2+) with configurable certificate/key
- Benchmarks for the hot paths, race-tested, `go vet` clean
- GitHub Actions CI (gofmt, vet, race tests, build, Docker build, benchmarks) and release automation

## Architecture

```
Client
   │
   ▼
Reverse Proxy (:8080)
   ├── Request ID → Access log/metrics → Rate limiter → Proxy handler
   │                                              │
   │                                    ┌─────────┴─────────┐
   │                                    │  Load balancer     │
   │                                    │  + circuit breaker │
   │                                    └─────────┬─────────┘
   ├── Admin API (/admin/*, token-protected)      │
   ├── Prometheus (/metrics)               ┌──────┴──────┐
   └── Health (/healthz)                   ▼             ▼
                                        Backend 1     Backend 2
                                        (health checks + failover)
```

Request flow: every request passes through the middleware chain (request ID, access logging, rate limiting) before the proxy handler selects a healthy, available backend through the configured load-balancing strategy. Failed upstream attempts are retried according to the retry policy; repeated failures open the backend's circuit breaker so traffic fails over instantly until a probe succeeds.

## Quick Start

### Docker Compose

```bash
cd reverse-proxy
ADMIN_TOKEN=change-me docker compose up --build
```

Endpoints:

| Endpoint | Description |
|---|---|
| `http://localhost:8080` | Reverse proxy |
| `http://localhost:8080/healthz` | Proxy liveness |
| `http://localhost:8080/metrics` | Prometheus metrics |
| `http://localhost:8080/admin/backends` | Admin API (requires token) |

Optional stacks:

```bash
docker compose --profile monitoring up -d    # Prometheus :9090, Grafana :3000
docker compose --profile redis up -d         # Redis for distributed rate limiting
```

### Local Development

```bash
cd reverse-proxy

go test ./...
go build -o proxy ./cmd/server
./proxy -config config/config.yaml
```

The default `config/config.yaml` expects backends on `localhost:8081` and `localhost:8082`:

```bash
docker run --rm -p 8081:80 nginx:alpine
docker run --rm -p 8082:80 nginx:alpine
```

## Configuration

`config/config.yaml` (all durations use Go duration syntax, e.g. `5s`, `1m30s`):

| Key | Default | Description |
|---|---|---|
| `listen` | `:8080` | Listen address |
| `backends[].url` | — | Backend base URL (`http`/`https`, host required; no credentials, query, or fragment) |
| `backends[].weight` | `1` | Weight for weighted round robin (1–100) |
| `load_balancing.strategy` | `round_robin` | `round_robin`, `weighted_round_robin`, `least_connections` |
| `health_check.interval` | `5s` | Time between health check rounds |
| `health_check.timeout` | `2s` | Per-probe timeout |
| `health_check.path` | `/health` | Health endpoint on backends |
| `health_check.healthy_threshold` | `2` | Consecutive successes to mark healthy |
| `health_check.unhealthy_threshold` | `3` | Consecutive failures to mark unhealthy |
| `rate_limit.requests_per_minute` | `100` | Sustained rate per client IP |
| `rate_limit.burst` | `50` | Token bucket burst per client IP |
| `rate_limit.trusted_proxies` | `[]` | CIDRs or IPs allowed to set `X-Forwarded-For` |
| `timeouts.backend_dial` | `3s` | Upstream dial timeout |
| `timeouts.backend_response_header` | `10s` | Upstream response header timeout |
| `timeouts.backend_idle_conn` | `90s` | Idle keep-alive lifetime |
| `timeouts.backend_tls_handshake` | `5s` | Upstream TLS handshake timeout |
| `timeouts.backend_max_idle_conns` | `256` | Upstream idle connection pool size |
| `timeouts.backend_max_idle_conns_per_host` | `64` | Per-backend idle connections |
| `timeouts.server_read` | `15s` | Inbound request read timeout |
| `timeouts.server_read_header` | `5s` | Inbound header read timeout |
| `timeouts.server_write` | `30s` | Response write timeout |
| `timeouts.server_idle` | `60s` | Keep-alive idle timeout |
| `timeouts.shutdown` | `10s` | Graceful shutdown budget |
| `retry.max_attempts` | `2` | Total attempts for idempotent bodyless requests (1–10) |
| `circuit_breaker.failure_threshold` | `5` | Failures before opening |
| `circuit_breaker.open_timeout` | `30s` | Time before half-open probe |
| `circuit_breaker.half_open_probes` | `1` | Successful probes required to close |
| `tls.cert_file` / `tls.key_file` | `""` | HTTPS termination (both required to enable) |
| `log.level` | `info` | `debug`, `info`, `warn`, `error` |
| `log.format` | `json` | `json` or `text` |
| `redis.addr` | `""` | Redis address enables distributed rate limiting |

### Environment Variables

| Variable | Purpose |
|---|---|
| `ADMIN_TOKEN` | Bearer token for the Admin API; when unset the admin API is disabled |
| `REDIS_PASSWORD` | Password for the Redis rate-limit backend |
| `GRAFANA_ADMIN_PASSWORD` | Grafana admin password (compose monitoring stack) |

## API

### Proxy

All unmatched paths are proxied to the selected backend. `X-Forwarded-For`, `X-Forwarded-Host`, and `X-Forwarded-Proto` are set on upstream requests.

### Operational Endpoints

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/metrics
```

### Admin API (requires `Authorization: Bearer $ADMIN_TOKEN`)

| Method | Path | Description |
|---|---|---|
| `GET` | `/admin/backends` | List backends with health, admin status, and in-flight counts |
| `POST` | `/admin/backends/{index}/down` | Remove a backend from rotation |
| `POST` | `/admin/backends/{index}/up` | Return a backend to rotation |

```bash
curl -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8080/admin/backends
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8080/admin/backends/0/down
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8080/admin/backends/0/up
```

Error responses use a consistent JSON envelope:

```json
{"error": {"code": "bad_gateway", "message": "upstream request failed"}, "request_id": "9f2c1a77b3e4d5a6"}
```

Codes: `rate_limit_exceeded` (429), `unauthorized` (401), `invalid_backend_index` (400), `bad_gateway` (502), `service_unavailable` (503), `gateway_timeout` (504).

## Metrics

| Metric | Type | Labels |
|---|---|---|
| `proxy_requests_total` | counter | `method`, `route`, `backend`, `status` |
| `proxy_request_duration_seconds` | histogram | `method`, `route`, `backend` |
| `proxy_inflight_requests` | gauge | — |
| `proxy_retries_total` | counter | — |
| `backend_requests_total` | counter | `backend` |
| `backend_errors_total` | counter | `backend` |
| `backend_timeouts_total` | counter | `backend` |
| `backend_healthy` | gauge | `backend` |
| `backend_circuit_breaker_open` | gauge | `backend` |
| `backend_healthchecks_total` | counter | `backend`, `result` |
| `rate_limit_rejected_total` | counter | — |
| `rate_limit_errors_total` | counter | — |

The `route` label is the first path segment, which bounds metric cardinality.

## Monitoring Stack

```bash
cd reverse-proxy
ADMIN_TOKEN=change-me docker compose --profile monitoring up -d
```

- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000` (admin / `GRAFANA_ADMIN_PASSWORD`, default `admin`)

The **Reverse Proxy** dashboard (`deploy/monitoring/grafana/dashboards/`) is provisioned automatically and covers request rate, latency percentiles, error rate, upstream traffic split, circuit breakers, in-flight requests, and health check results.

## Security Notes

- Set a strong `ADMIN_TOKEN`; without it the admin API is not served at all
- Configure `rate_limit.trusted_proxies` before trusting `X-Forwarded-For`; otherwise client-supplied forwarding headers are ignored and only the socket address is used
- Rate limiting fails open if Redis is unavailable (availability over strict limiting); failures are counted in `rate_limit_errors_total`
- Retries are only issued for idempotent requests without a body, so uploads are never duplicated
- The container runs as a non-root user with a read-only root filesystem and no capabilities; keep TLS certificates mounted read-only
- `/metrics` is unauthenticated by design for scrape simplicity; restrict it at the network layer if the proxy is internet-facing

## Testing and Benchmarks

```bash
go vet ./...
go test -race -count=1 ./...
go test -run '^$' -bench=. -benchmem ./...
go test -run '^$' -bench=BenchmarkProxyServeHTTPParallel -cpuprofile=cpu.out -memprofile=mem.out ./internal/proxy/
```

Integration tests (`internal/app`) cover proxying, failover and retry, backend failure responses, rate limiting, admin authentication and traffic control, health-check recovery, and operational endpoints, all against in-memory `httptest` servers.

Benchmark results (Go 1.27, linux/amd64, 2 vCPU sandbox):

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| `RoundRobinNext` (parallel) | 47 | 0 | 0 |
| `WeightedRoundRobinNext` (parallel) | 52 | 0 | 0 |
| `LeastConnectionsNext` (parallel) | 85 | 0 | 0 |
| `BackendState` (parallel) | 39 | 0 | 0 |
| `MemoryAllowSingleKey` (parallel) | 189 | 0 | 0 |
| `MemoryAllowManyKeys` (parallel) | 197 | 2 | 0 |
| `ProxyServeHTTP` (end-to-end hop) | 61 µs | 40 KB | 93 |

Profiling shows request cost is dominated by `net/http` internals (request cloning, MIME header parsing, socket syscalls); the load balancer, breaker, and limiter paths are allocation-free. Remaining proxy allocations come from `httputil.ReverseProxy` itself and are not worth reimplementing.

## Deployment

### Docker

```bash
cd reverse-proxy
docker build -t reverse-proxy .
docker run --rm -p 8080:8080 -e ADMIN_TOKEN=change-me reverse-proxy
```

### HTTPS Termination

```yaml
tls:
  cert_file: /etc/tls/server.crt
  key_file: /etc/tls/server.key
```

Both files must exist or the proxy refuses to start. Terminate public TLS at the proxy or at a CDN/load balancer in front of it; set `rate_limit.trusted_proxies` to the load balancer's addresses.

### Distributed Rate Limiting

```yaml
redis:
  addr: redis:6379
```

with `REDIS_PASSWORD` in the environment. Limits are enforced atomically in Redis with a Lua token bucket, so multiple proxy instances share one budget.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `admin API is disabled` | `ADMIN_TOKEN` unset | Export `ADMIN_TOKEN` and restart |
| `503 service_unavailable` | No healthy/available backend | `GET /admin/backends`, check health checks and circuit breakers |
| `502 bad_gateway` | Upstream unreachable or refused | Verify backend URLs and network reachability |
| `504 gateway_timeout` | Upstream exceeded `timeouts.backend_response_header` | Increase the timeout or investigate slow backends |
| `429 rate_limit_exceeded` | Client over limit | Tune `rate_limit.requests_per_minute` / `burst` |
| Clients appear as load-balancer IPs | Missing trusted proxy config | Set `rate_limit.trusted_proxies` |
| Config rejected at startup | Validation failure | The error lists every invalid field |
| Redis errors in `rate_limit_errors_total` | Redis unreachable | Check `redis.addr` / `REDIS_PASSWORD`; traffic continues (fail-open) |

## Project Structure

```
reverse-proxy/
├── cmd/server/             # entrypoint (flags, signals, wiring)
├── internal/
│   ├── admin/              # token-authenticated admin API
│   ├── app/                # dependency wiring + integration tests
│   ├── balancer/           # backends + pickers (RR, WRR, least-conn)
│   ├── breaker/            # circuit breaker state machine
│   ├── clientip/           # trusted-proxy client IP resolution
│   ├── config/             # YAML loading + validation
│   ├── health/             # active health checking
│   ├── httpx/              # request IDs, error envelopes, status capture
│   ├── limiter/            # memory + Redis rate limiters
│   ├── logging/            # slog setup + access logging
│   ├── metrics/            # Prometheus instrumentation
│   ├── proxy/              # reverse proxy handler, retries, error mapping
│   └── server/             # HTTP(S) server + graceful shutdown
├── config/                 # example configurations (local + docker)
├── deploy/
│   ├── backend/            # demo nginx backends
│   └── monitoring/         # Prometheus + Grafana provisioning
├── Dockerfile
└── docker-compose.yml
```

## Roadmap Status

All milestones from the project roadmap are implemented:

- **M1 Reliability** — error handling, configurable timeouts, validated config, thread-safe backend state, graceful shutdown, full unit + integration test suite (`go test -race`, `go vet`)
- **M2 Security** — admin authentication, trusted-proxy client IP handling, request validation, environment-based secrets, secure defaults
- **M3 Observability** — `log/slog` structured logging, extended Prometheus metrics, Prometheus + Grafana stack, pre-built dashboard
- **M4 Resilience** — circuit breaker, configurable retries, weighted round robin, least connections, optional Redis-backed distributed rate limiting
- **M5 Production Readiness** — TLS termination, upstream connection pooling, benchmarks and profiling, hardened Docker image, CI/CD with automated tests, Docker builds, and tagged releases
- **M6 Release** — complete documentation, deployment examples, troubleshooting guide, reproducible benchmarks

## Tech Stack

- Go 1.26+
- `net/http` + `net/http/httputil`
- `log/slog`
- `golang.org/x/time/rate`
- `github.com/prometheus/client_golang`
- `github.com/redis/go-redis/v9`
- `gopkg.in/yaml.v3`

## License

MIT
