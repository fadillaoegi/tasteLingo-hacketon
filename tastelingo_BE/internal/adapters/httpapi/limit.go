package httpapi

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Limiter protects paid model and Qloo quota on the public demo. It allows
// PerClient requests per Window for each client IP, and Global requests per
// Window across all clients. A zero limit disables that check.
type Limiter struct {
	PerClient  int
	Global     int
	Window     time.Duration
	TrustProxy bool
	Now        func() time.Time

	mu      sync.Mutex
	windows map[string]*window
}

type window struct {
	start time.Time
	count int
}

const globalKey = "*"

func (l *Limiter) Wrap(next http.HandlerFunc) http.HandlerFunc {
	if l == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if wait, ok := l.allow(l.clientIP(r)); !ok {
			minutes := int(wait.Minutes()) + 1
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			write(w, http.StatusTooManyRequests, map[string]string{"error": fmt.Sprintf("The demo is busy or you reached its limit. Please try again in about %d minute(s).", minutes)})
			return
		}
		next(w, r)
	}
}

// allow counts one request for the client and globally, or reports how long to wait.
func (l *Limiter) allow(client string) (time.Duration, bool) {
	now := time.Now()
	if l.Now != nil {
		now = l.Now()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.windows == nil {
		l.windows = map[string]*window{}
	}
	if len(l.windows) > 10000 {
		for k, v := range l.windows {
			if now.Sub(v.start) >= l.Window {
				delete(l.windows, k)
			}
		}
	}
	check := func(key string, limit int) (*window, time.Duration, bool) {
		if limit <= 0 {
			return nil, 0, true
		}
		win := l.windows[key]
		if win == nil || now.Sub(win.start) >= l.Window {
			win = &window{start: now}
			l.windows[key] = win
		}
		if win.count >= limit {
			return nil, win.start.Add(l.Window).Sub(now), false
		}
		return win, 0, true
	}
	cw, wait, ok := check("ip:"+client, l.PerClient)
	if !ok {
		return wait, false
	}
	gw, wait, ok := check(globalKey, l.Global)
	if !ok {
		return wait, false
	}
	// Count only after both checks pass, so a rejected request uses no quota.
	if cw != nil {
		cw.count++
	}
	if gw != nil {
		gw.count++
	}
	return 0, true
}

// clientIP uses the right-most X-Forwarded-For entry when behind a trusted
// proxy, because that entry is appended by the proxy and cannot be spoofed.
func (l *Limiter) clientIP(r *http.Request) string {
	if l.TrustProxy {
		parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		for i := len(parts) - 1; i >= 0; i-- {
			if ip := strings.TrimSpace(parts[i]); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
