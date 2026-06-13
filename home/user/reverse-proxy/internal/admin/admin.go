package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"reverse-proxy/internal/balancer"
)

type BackendInfo struct {
	Index int    `json:"index"`
	URL   string `json:"url"`
	Alive bool   `json:"alive"`
}

type Admin struct {
	backends []*balancer.Backend
}

func NewAdmin(backends []*balancer.Backend) *Admin {
	return &Admin{backends: backends}
}

func (a *Admin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin")

	switch {
	case path == "/backends" && r.Method == http.MethodGet:
		a.listBackends(w, r)
	case strings.HasPrefix(path, "/backends/") && strings.HasSuffix(path, "/down") && r.Method == http.MethodPost:
		a.setBackendStatus(w, r, false)
	case strings.HasPrefix(path, "/backends/") && strings.HasSuffix(path, "/up") && r.Method == http.MethodPost:
		a.setBackendStatus(w, r, true)
	default:
		http.NotFound(w, r)
	}
}

func (a *Admin) listBackends(w http.ResponseWriter, r *http.Request) {
	info := make([]BackendInfo, len(a.backends))
	for i, b := range a.backends {
		info[i] = BackendInfo{
			Index: i,
			URL:   b.URL.String(),
			Alive: b.IsAlive(),
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (a *Admin) setBackendStatus(w http.ResponseWriter, r *http.Request, alive bool) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	idxStr := parts[3] // /admin/backends/{index}/down
	idx, err := strconv.Atoi(idxStr)
	if err != nil || idx < 0 || idx >= len(a.backends) {
		http.Error(w, "invalid backend index", http.StatusBadRequest)
		return
	}

	a.backends[idx].SetAlive(alive)

	status := "down"
	if alive {
		status = "up"
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("backend " + strconv.Itoa(idx) + " set to " + status + "\n"))
}
