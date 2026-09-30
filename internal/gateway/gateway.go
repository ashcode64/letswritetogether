package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"hash/fnv"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"colorbook/internal/auth"
	"colorbook/internal/live"
	"colorbook/internal/problem"
	"colorbook/web"
)

type Service struct {
	RDB     *redis.Client
	Secret  string
	Secure  bool
	Proxy   *httputil.ReverseProxy
	lim     *limiter
}

func New(rdb *redis.Client, secret, proxyURL string, secure bool) *Service {
	target, err := url.Parse(proxyURL)
	if err != nil {
		panic(err)
	}
	p := httputil.NewSingleHostReverseProxy(target)
	p.FlushInterval = -1
	p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		problem.Write(w, http.StatusBadGateway, "UPSTREAM", "the next service did not answer")
	}
	return &Service{RDB: rdb, Secret: secret, Secure: secure, Proxy: p, lim: newLimiter()}
}

func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/sessions", s.session)
	mux.HandleFunc("GET /v1/rounds/current", s.current)
	mux.HandleFunc("POST /v1/rounds/{id}/words", s.forward("submit", 30))
	mux.HandleFunc("GET /v1/rounds/{id}/words", s.forward("read", 120))
	mux.HandleFunc("PUT /v1/rounds/{id}/vote", s.forward("vote", 60))
	mux.HandleFunc("GET /v1/canvas/", s.forward("read", 120))
	mux.HandleFunc("GET /v1/canvas/today", s.forward("read", 120))
	mux.HandleFunc("GET /v1/events", s.forward("events", 60))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /{$}", s.file("index.html", "text/html; charset=utf-8"))
	mux.HandleFunc("GET /app.js", s.file("app.js", "text/javascript; charset=utf-8"))
	mux.HandleFunc("GET /app.css", s.file("app.css", "text/css; charset=utf-8"))
}

func (s *Service) file(name, ctype string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := web.Files.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(b)
	}
}

func (s *Service) session(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.lim.allow(ip+":session", 10, time.Minute) {
		w.Header().Set("Retry-After", "2")
		problem.Write(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many new sessions")
		return
	}
	did, err := auth.FromRequest(r, s.Secret)
	if err != nil {
		buf := make([]byte, 16)
		if _, err := rand.Read(buf); err != nil {
			problem.Write(w, http.StatusInternalServerError, "UNAVAILABLE", "could not mint a device")
			return
		}
		did = hex.EncodeToString(buf)
	}
	auth.SetCookie(w, s.Secret, did, s.Secure)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) current(w http.ResponseWriter, r *http.Request) {
	did, err := auth.FromRequest(r, s.Secret)
	if err != nil {
		problem.Write(w, http.StatusUnauthorized, "UNAUTHORIZED", "session required")
		return
	}
	snap, err := live.ReadPhase(r.Context(), s.RDB)
	if err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "clock unavailable")
		return
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(did))
	_, _ = h.Write([]byte(strconv.FormatInt(snap.RoundID, 10)))
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"round_id":       snap.RoundID,
		"phase":          snap.Phase,
		"server_time":    snap.ServerTime,
		"server_time_ms": snap.ServerTimeMS,
		"submit_ms_left": snap.SubmitMSLeft,
		"vote_ms_left":   snap.VoteMSLeft,
		"shuffle_seed":   h.Sum32(),
	})
}

func (s *Service) forward(kind string, perMinute int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		did, err := auth.FromRequest(r, s.Secret)
		if err != nil {
			problem.Write(w, http.StatusUnauthorized, "UNAUTHORIZED", "session required")
			return
		}
		if !s.lim.allow(clientIP(r)+":"+kind, perMinute, time.Minute) {
			w.Header().Set("Retry-After", "1")
			problem.Write(w, http.StatusTooManyRequests, "RATE_LIMITED", "slow down")
			return
		}
		r.Header.Del("X-Internal-Token")
		r.Header.Set("X-Did", did)
		s.Proxy.ServeHTTP(w, r)
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type limiter struct {
	mu sync.Mutex
	m  map[string]*win
}

type win struct {
	start time.Time
	n     int
}

func newLimiter() *limiter { return &limiter{m: map[string]*win{}} }

func (l *limiter) allow(key string, max int, dur time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b := l.m[key]
	if b == nil || now.Sub(b.start) >= dur {
		l.m[key] = &win{start: now, n: 1}
		return true
	}
	if b.n >= max {
		return false
	}
	b.n++
	return true
}
