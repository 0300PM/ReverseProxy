# Reverse Proxy – Iteration 2

Lepsza wersja MVP z observability i operacyjnym API.

## Nowe rzeczy w Iteration 2

- **Admin API**
  - `GET /admin/backends` → JSON z listą + statusem Alive
  - `POST /admin/backends/{index}/down`
  - `POST /admin/backends/{index}/up`

- **Prometheus Metrics** (`/metrics`)
  - `proxy_requests_total{method,path,backend,status}`
  - `proxy_request_duration_seconds` (histogram)
  - `backend_healthy` (gauge 0/1)
  - `rate_limit_rejected_total`

- **Lepsze Health Checks**
  - Konfigurowalna ścieżka (`health_check_path`)
  - `HEAD` zamiast `GET`
  - Logowanie **tylko zmian stanu** (nie spam co 5s)

- Podstawowe testy jednostkowe (`balancer/*_test.go`)

## Uruchomienie

```bash
docker-compose up --build
```

- Proxy:   http://localhost:8080
- Admin:   http://localhost:8080/admin/backends
- Metrics: http://localhost:8080/metrics

### Przykłady

```bash
# Lista backendów
curl http://localhost:8080/admin/backends

# Wyłącz backend 0
curl -X POST http://localhost:8080/admin/backends/0/down

# Włącz z powrotem
curl -X POST http://localhost:8080/admin/backends/0/up

# Metryki
curl -s http://localhost:8080/metrics | grep -E 'proxy_requests_total|backend_healthy|rate_limit'
```

## Konfiguracja

```yaml
port: 8080

backends:
  - http://localhost:8081
  - http://localhost:8082

health_check_interval: 5s
health_check_path: /health     # <--- nowa opcja

rate_limit:
  requests_per_minute: 100
```

## Co pokazuje na rozmowie (po Iteration 2)

- Wszystko z MVP +
- **Observability** (Prometheus)
- **Control plane** (Admin API)
- Operacyjność (można ręcznie wyłączać backendy)
- Lepsze testowanie i health checking

## Struktura

```
reverse-proxy/
├── cmd/server/main.go
├── internal/
│   ├── admin/admin.go
│   ├── balancer/
│   │   ├── backend.go
│   │   ├── backend_test.go
│   │   └── round_robin.go
│   │   └── round_robin_test.go
│   ├── health/checker.go
│   ├── metrics/metrics.go
│   ├── proxy/proxy.go
│   ├── ratelimit/limiter.go
│   └── logger/logger.go
├── config/config.yaml
├── docker-compose.yml
├── Dockerfile
└── README.md
```

## Następna iteracja (propozycja)

- Weighted Round Robin / Least Connections
- Circuit Breaker
- Structured logging (slog)
- Lepsze testy integracyjne

~550-650 linii Go (w tym testy).