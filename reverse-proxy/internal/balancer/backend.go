package balancer

import (
	"net/http/httputil"
	"net/url"
	"sync"
)

type Backend struct {
	URL   *url.URL
	Alive bool
	Proxy *httputil.ReverseProxy
	Mutex sync.RWMutex
}

func NewBackend(rawURL string) (*Backend, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(u)

	return &Backend{
		URL:   u,
		Alive: true,
		Proxy: proxy,
	}, nil
}

func (b *Backend) SetAlive(alive bool) {
	b.Mutex.Lock()
	defer b.Mutex.Unlock()
	b.Alive = alive
}

func (b *Backend) IsAlive() bool {
	b.Mutex.RLock()
	defer b.Mutex.RUnlock()
	return b.Alive
}
