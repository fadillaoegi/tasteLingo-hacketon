package httpapi

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// spa serves the built frontend. Unknown paths fall back to index.html so a
// reload on any client-side route still opens the app.
func spa(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + r.URL.Path)
		if info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(clean))); err != nil || info.IsDir() {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		if strings.HasPrefix(clean, "/assets/") {
			// Vite fingerprints asset names, so they never change in place.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

// Mount serves h under base, such as /tastelingo, when a reverse proxy forwards
// a subpath without stripping it. An empty base serves h from the root.
func Mount(base string, h http.Handler) http.Handler {
	base = strings.TrimRight(base, "/")
	if base == "" {
		return h
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	inner := http.StripPrefix(base, h)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == base {
			http.Redirect(w, r, base+"/", http.StatusFound)
			return
		}
		inner.ServeHTTP(w, r)
	})
}

// secure adds headers that are safe for both the API and the frontend.
func secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
