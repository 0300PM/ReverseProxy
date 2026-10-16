package clientip

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func req(remoteAddr string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remoteAddr
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestNoTrustedProxies(t *testing.T) {
	r, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	got := r.FromRequest(req("203.0.113.9:4242", map[string]string{"X-Forwarded-For": "1.2.3.4"}))
	if got != "203.0.113.9" {
		t.Fatalf("got %q", got)
	}
}

func TestTrustedProxyForwardedChain(t *testing.T) {
	r, err := New([]string{"10.0.0.0/8", "192.168.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	got := r.FromRequest(req("10.1.2.3:80", map[string]string{
		"X-Forwarded-For": "203.0.113.7, 192.168.1.1, 10.9.9.9",
	}))
	if got != "203.0.113.7" {
		t.Fatalf("got %q", got)
	}
}

func TestTrustedProxySingleIP(t *testing.T) {
	r, err := New([]string{"192.168.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	got := r.FromRequest(req("192.168.1.1:80", map[string]string{"X-Real-IP": "203.0.113.7"}))
	if got != "203.0.113.7" {
		t.Fatalf("got %q", got)
	}
}

func TestUntrustedCannotSpoof(t *testing.T) {
	r, err := New([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	got := r.FromRequest(req("203.0.113.1:80", map[string]string{"X-Forwarded-For": "1.2.3.4", "X-Real-IP": "5.6.7.8"}))
	if got != "203.0.113.1" {
		t.Fatalf("got %q", got)
	}
}

func TestMalformedForwardedChain(t *testing.T) {
	r, err := New([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	got := r.FromRequest(req("10.1.2.3:80", map[string]string{"X-Forwarded-For": "garbage, 10.1.1.1"}))
	if got != "10.1.2.3" {
		t.Fatalf("got %q", got)
	}
}

func TestAllTrustedChain(t *testing.T) {
	r, err := New([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	got := r.FromRequest(req("10.1.2.3:80", map[string]string{"X-Forwarded-For": "10.2.2.2, 10.3.3.3"}))
	if got != "10.2.2.2" {
		t.Fatalf("got %q", got)
	}
}

func TestInvalidConfig(t *testing.T) {
	if _, err := New([]string{"nope"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRemoteAddrWithoutPort(t *testing.T) {
	r, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	got := r.FromRequest(req("203.0.113.9", nil))
	if got != "203.0.113.9" {
		t.Fatalf("got %q", got)
	}
}
