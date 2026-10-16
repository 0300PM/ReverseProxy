package admin

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"reverse-proxy/internal/balancer"
	"reverse-proxy/internal/httpx"
)

type BackendInfo struct {
	Index     int    `json:"index"`
	URL       string `json:"url"`
	Healthy   bool   `json:"healthy"`
	AdminDown bool   `json:"admin_down"`
	Inflight  int64  `json:"inflight"`
}

type statusResponse struct {
	Index  int    `json:"index"`
	Status string `json:"status"`
}

type Admin struct {
	handler   http.Handler
	backends  []*balancer.Backend
	tokenHash [sha256.Size]byte
	logger    *slog.Logger
}

func New(backends []*balancer.Backend, token string, logger *slog.Logger) *Admin {
	a := &Admin{
		backends:  backends,
		tokenHash: sha256.Sum256([]byte(token)),
		logger:    logger,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/backends", a.listBackends)
	mux.HandleFunc("POST /admin/backends/{index}/up", a.backendStatusHandler(false))
	mux.HandleFunc("POST /admin/backends/{index}/down", a.backendStatusHandler(true))
	a.handler = a.authenticate(mux)
	return a
}

func (a *Admin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	a.handler.ServeHTTP(w, r)
}

func (a *Admin) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.authorized(r) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="admin"`)
			httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "missing or invalid admin token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Admin) authorized(r *http.Request) bool {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	got := sha256.Sum256([]byte(auth[len(prefix):]))
	return subtle.ConstantTimeCompare(a.tokenHash[:], got[:]) == 1
}

func (a *Admin) listBackends(w http.ResponseWriter, r *http.Request) {
	infos := make([]BackendInfo, len(a.backends))
	for i, b := range a.backends {
		infos[i] = BackendInfo{
			Index:     i,
			URL:       b.String(),
			Healthy:   b.Healthy(),
			AdminDown: b.AdminDown(),
			Inflight:  b.Inflight(),
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(infos)
}

func (a *Admin) backendStatusHandler(down bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idx, err := strconv.Atoi(r.PathValue("index"))
		if err != nil || idx < 0 || idx >= len(a.backends) {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_backend_index", "no backend with that index")
			return
		}
		b := a.backends[idx]
		b.SetAdminDown(down)
		status := "up"
		if down {
			status = "down"
		}
		a.logger.Info("admin backend status change",
			"backend", b.String(),
			"status", status,
			"request_id", httpx.RequestIDFrom(r.Context()),
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(statusResponse{Index: idx, Status: status})
	}
}
