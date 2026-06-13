package balancer

import (
	"net/http/httputil"
	"net/url"
	"testing"
)

func TestBackend_IsAlive_SetAlive(t *testing.T) {
	u, _ := url.Parse("http://example.com")
	b := &Backend{
		URL:   u,
		Alive: true,
		Proxy: httputil.NewSingleHostReverseProxy(u),
	}

	if !b.IsAlive() {
		t.Error("expected initially alive")
	}

	b.SetAlive(false)
	if b.IsAlive() {
		t.Error("expected dead after SetAlive(false)")
	}

	b.SetAlive(true)
	if !b.IsAlive() {
		t.Error("expected alive after SetAlive(true)")
	}
}
