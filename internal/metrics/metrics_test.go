package metrics

import "testing"

func TestRoute(t *testing.T) {
	cases := map[string]string{
		"":               "/",
		"/":              "/",
		"/api":           "/api",
		"/api/":          "/api",
		"/api/v1/users":  "/api",
		"/healthz":       "/healthz",
		"metrics":        "/metrics",
		"/a/b/c/d/e/f/g": "/a",
	}
	for in, want := range cases {
		if got := Route(in); got != want {
			t.Errorf("Route(%q) = %q, want %q", in, got, want)
		}
	}
}
