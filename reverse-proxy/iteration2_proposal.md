# Propozycja Iteration 2

## Aktualny stan (MVP)
- 322 linie Go
- Round Robin + atomics
- Health checks + failover
- Rate limiting + logging
- Działa z Dockerem (nginx jako backendy)
- Kompiluje się i uruchamia

## Rekomendowana Iteration 2 (najlepszy stosunek wartości do wysiłku)

### Główne rzeczy do dodania:

1. **Admin API** (najważniejsze)
   - `GET /admin/backends` — lista backendów ze statusem (JSON)
   - `POST /admin/backends/{index}/down`
   - `POST /admin/backends/{index}/up`
   - Opcjonalnie: force health check

   → To jest złoto na rozmowie. Możesz na żywo wyłączyć backend i pokazać, że ruch leci na drugi.

2. **Prometheus Metrics**
   - Endpoint `/metrics`
   - Kluczowe metryki:
     - `proxy_requests_total`
     - `proxy_request_duration_seconds` (histogram)
     - `backend_healthy` (gauge)
     - `rate_limit_rejected_total`
   - Middleware zbierający metryki

3. **Lepsze Health Checks**
   - Konfigurowalna ścieżka health (`health_check_path`)
   - Używanie `HEAD` zamiast `GET`
   - Logowanie tylko zmian stanu (nie spam co 5s)

4. **Podstawowe testy jednostkowe**
   - `balancer/round_robin_test.go`
   - Proste testy Backend

### Co dokładnie zmienimy / dodamy:
- Nowy pakiet: `internal/admin/`
- Nowy pakiet lub rozszerzenie: metryki (można w `internal/metrics/`)
- Modyfikacja `health/checker.go`
- Modyfikacja `cmd/server/main.go` (nowe endpointy + rejestracja metryk)
- Aktualizacja `config.yaml`
- Aktualizacja `README.md` + `docker-compose.yml` (prometheus + grafana opcjonalnie)

### Co świadomie pomijamy w tej iteracji:
- Drugi algorytm load balancingu (Weighted / Least Connections) → Iteration 3
- Structured logging (slog)
- Circuit breaker
- TLS
- Pełne testy integracyjne z httptest

## Dlaczego właśnie to?

To jest obecnie najmocniejszy możliwy krok po czystym MVP:

- Pokazuje **operacyjność** i myślenie o produkcji (Admin API + metrics)
- Admin API + metryki to tematy, które bardzo często padają na rozmowach Go Backend / SRE / Platform
- Łatwo i efektownie się demo'uje
- Nie rozdmuchujemy projektu za bardzo (realistycznie da się zrobić w 1 tydzień)
- Całość nadal mieści się w 600-900 liniach

## Alternatywy (jeśli wolisz inną kolejność)

A. Najpierw drugi algorytm load balancingu (Least Connections lub Weighted)
B. Najpierw testy + lepszy health check (mniejszy zakres)
C. Structured logging + testy
D. Coś innego (napisz co)

## Pytanie do Ciebie

Idziemy w **Admin API + Metrics + Lepsze Health Checks + podstawowe testy**?

Czy chcesz zmienić zakres / kolejność?
