# Reverse Proxy MVP

Prosty, ale kompletny reverse proxy w Go pokazujący najważniejsze koncepcje backendowe.

## Architektura

```
Client --> Reverse Proxy (8080) --> [Backend #1 (8081), Backend #2 (8082)]
```

- **Load Balancing**: Round Robin
- **Health Checks**: Co 5 sekund, automatyczny failover
- **Rate Limiting**: IP-based (100 req/min domyślnie)
- **Logging**: Proste request logging

## Features (MVP)

- Reverse Proxy (używając `httputil.NewSingleHostReverseProxy`)
- Round Robin Load Balancing z atomics
- Health Checks + Automatic Failover
- IP-based Rate Limiting (`golang.org/x/time/rate`)
- Request Logging (middleware)

## Uruchomienie (Docker - zalecane)

```bash
docker-compose up --build
```

Proxy będzie dostępne na `http://localhost:8080`

Backendy (nginx) na 8081 i 8082.

### Testowanie

```bash
# Proste requesty
curl http://localhost:8080
curl http://localhost:8080

# Sprawdź logi proxy
docker-compose logs -f proxy

# Symulacja awarii backendu
docker-compose stop backend1

# Cały ruch powinien iść na backend2
curl -I http://localhost:8080
```

## Uruchomienie lokalne (bez Dockera)

```bash
# Terminal 1 - Backend 1
docker run --rm -p 8081:80 nginx:alpine

# Terminal 2 - Backend 2
docker run --rm -p 8082:80 nginx:alpine

# Terminal 3 - Proxy
go run ./cmd/server
```

## Konfiguracja

Plik `config/config.yaml`:

```yaml
port: 8080

backends:
  - http://localhost:8081
  - http://localhost:8082

health_check_interval: 5s

rate_limit:
  requests_per_minute: 100
```

## Co pokazuje na rozmowie technicznej

- **Reverse Proxy** i jak działa `httputil`
- **Load Balancing** (Round Robin + atomic operations)
- **Concurrency** (goroutines + mutexy w Backend)
- **Middleware** pattern w Go
- **Health Checks** i failover
- **Rate Limiting** z token bucket (`golang.org/x/time/rate`)
- **Structy** z sync primitives

## Struktura projektu

```
reverse-proxy/
├── cmd/server/main.go
├── internal/
│   ├── balancer/
│   │   ├── backend.go
│   │   └── round_robin.go
│   ├── health/checker.go
│   ├── proxy/proxy.go
│   ├── ratelimit/limiter.go
│   └── logger/logger.go
├── config/config.yaml
├── docker-compose.yml
├── Dockerfile
└── README.md
```

## Future Improvements (po MVP)

- Weighted Round Robin / Least Connections
- Prometheus metrics + /metrics endpoint
- TLS termination
- Admin API (list backends, manual health toggle)
- Circuit Breaker
- Connection pooling / keep-alive tuning
- gRPC support
- Redis-backed rate limiting (dla wielu instancji)

## Linie kodu

~320 linii czystego Go (bez vendorów) + konfiguracja.

Idealny projekt na portfolio i rozmowę techniczną na stanowisko Go Backend / Platform Engineer.
