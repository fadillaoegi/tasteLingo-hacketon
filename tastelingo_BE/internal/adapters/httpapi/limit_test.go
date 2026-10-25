package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLimiterPerClientGlobalAndWindow(t *testing.T) {
	now := time.Unix(0, 0)
	l := &Limiter{PerClient: 2, Global: 3, Window: time.Hour, TrustProxy: true, Now: func() time.Time { return now }}
	ok := l.Wrap(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	hit := func(xff string) int {
		r := httptest.NewRequest("POST", "/", nil)
		r.Header.Set("X-Forwarded-For", xff)
		rec := httptest.NewRecorder()
		ok(rec, r)
		return rec.Code
	}
	// The right-most entry is the proxy-observed client; a spoofed left entry must not help.
	if hit("1.1.1.1, 10.0.0.1") != 204 || hit("9.9.9.9, 10.0.0.1") != 204 || hit("10.0.0.1") != 429 {
		t.Fatal("per-client limit must use the proxy-appended address")
	}
	if hit("10.0.0.2") != 204 || hit("10.0.0.3") != 429 {
		t.Fatal("global limit must stop new clients")
	}
	now = now.Add(time.Hour)
	if hit("10.0.0.1") != 204 {
		t.Fatal("limits must reset after the window")
	}
}

func TestRejectedRequestHasRetryAfter(t *testing.T) {
	l := &Limiter{PerClient: 1, Window: time.Minute}
	h := l.Wrap(func(http.ResponseWriter, *http.Request) {})
	for i, want := range []int{200, 429} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest("GET", "/", nil))
		if rec.Code != want || i == 1 && rec.Header().Get("Retry-After") == "" {
			t.Fatalf("request %d: code %d, retry-after %q", i, rec.Code, rec.Header().Get("Retry-After"))
		}
	}
}

func TestStaticFallbackAndUnknownAPI(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "assets", "a.js"), []byte("js"), 0o644)
	h := (&Handler{StaticDir: dir}).Routes()
	get := func(p string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		return rec
	}
	if rec := get("/some/route"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "app") {
		t.Fatalf("client routes should get index.html: %d", rec.Code)
	}
	if rec := get("/assets/a.js"); rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatal("assets should be cached")
	}
	if rec := get("/../../etc/passwd"); strings.Contains(rec.Body.String(), "root:") {
		t.Fatal("path traversal")
	}
	if rec := get("/api/nope"); rec.Code != 404 || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
		t.Fatalf("unknown API path should be JSON 404, got %d", rec.Code)
	}
	if get("/").Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers missing")
	}
}
