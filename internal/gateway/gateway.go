package gateway

import (
	"crypto/ed25519"
	"encoding/json"
	"hash/fnv"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"colorbook/internal/live"
	"colorbook/internal/problem"
	"colorbook/internal/svcjwt"
	"colorbook/web"
)

type Service struct {
	RDB             *redis.Client
	Secret          string
	Secure          bool
	TrustCF         bool
	TurnstileSecret string
	TurnstileSite   string
	PowMax          int
	Priv            ed25519.PrivateKey
	Proxy           *httputil.ReverseProxy
	ProxyHost       string
	lim             *limiter
}

// New builds the gateway. proxyHost, when set (PROXY_HOST), is forced on every
// reverse-proxy hop so Mendr matches the private ingress table (sharedpage-internal).
func New(rdb *redis.Client, secret, proxyURL, proxyHost string, secure bool, priv ed25519.PrivateKey, turnstileSecret, turnstileSite string, powMax int) *Service {
	target, err := url.Parse(proxyURL)
	if err != nil {
		panic(err)
	}
	proxyHost = strings.TrimSpace(proxyHost)
	p := httputil.NewSingleHostReverseProxy(target)
	p.FlushInterval = -1
	baseDirector := p.Director
	p.Director = func(req *http.Request) {
		baseDirector(req)
		if proxyHost != "" {
			req.Host = proxyHost
			req.Header.Set("Host", proxyHost)
		}
	}
	p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		problem.Write(w, http.StatusBadGateway, "UPSTREAM", "the next service did not answer")
	}
	if powMax < 1 {
		powMax = 20000
	}
	return &Service{
		RDB: rdb, Secret: secret, Secure: secure, TrustCF: os.Getenv("TRUST_CF_CONNECTING_IP") == "1",
		TurnstileSecret: turnstileSecret, TurnstileSite: turnstileSite, PowMax: powMax,
		Priv: priv, Proxy: p, ProxyHost: proxyHost, lim: newLimiter(),
	}
}

func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/sessions/config", s.sessionConfig)
	mux.HandleFunc("POST /v1/sessions/challenge", s.challenge)
	mux.HandleFunc("POST /v1/sessions", s.session)
	mux.HandleFunc("POST /v1/sessions/refresh", s.refresh)
	mux.HandleFunc("GET /v1/rounds/current", s.current)
	mux.HandleFunc("POST /v1/rounds/{id}/words", s.forward("submit", 30, svcjwt.AudSubmission))
	mux.HandleFunc("GET /v1/rounds/{id}/words", s.forward("read", 120, svcjwt.AudSubmission))
	mux.HandleFunc("PUT /v1/rounds/{id}/vote", s.forward("vote", 60, svcjwt.AudVoting))
	mux.HandleFunc("GET /v1/canvas/", s.public("read", 120))
	mux.HandleFunc("GET /v1/canvas/today", s.public("read", 120))
	mux.HandleFunc("GET /v1/events", s.public("events", 60))
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
		if name == "index.html" {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://challenges.cloudflare.com; frame-src https://challenges.cloudflare.com; worker-src 'self' blob:; connect-src 'self' https://challenges.cloudflare.com; style-src 'self'; img-src 'self'; base-uri 'self'; frame-ancestors 'none'")
		}
		_, _ = w.Write(b)
	}
}

func (s *Service) current(w http.ResponseWriter, r *http.Request) {
	did, err := s.device(r)
	if err != nil {
		writeDeviceErr(w, err)
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

func (s *Service) forward(kind string, perMinute int, aud string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		did, err := s.device(r)
		if err != nil {
			writeDeviceErr(w, err)
			return
		}
		if !s.lim.allow(s.clientIP(r)+":"+kind, perMinute, time.Minute) {
			w.Header().Set("Retry-After", "1")
			problem.Write(w, http.StatusTooManyRequests, "RATE_LIMITED", "slow down")
			return
		}
		tok, err := s.bearer(aud, did)
		if err != nil {
			problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "could not sign the request")
			return
		}
		r.Header.Del("X-Internal-Token")
		r.Header.Del("X-Did")
		r.Header.Set("Authorization", "Bearer "+tok)
		s.Proxy.ServeHTTP(w, r)
	}
}

func (s *Service) public(kind string, perMinute int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.lim.allow(s.clientIP(r)+":"+kind, perMinute, time.Minute) {
			w.Header().Set("Retry-After", "1")
			problem.Write(w, http.StatusTooManyRequests, "RATE_LIMITED", "slow down")
			return
		}
		r.Header.Del("Authorization")
		r.Header.Del("X-Internal-Token")
		r.Header.Del("X-Did")
		s.Proxy.ServeHTTP(w, r)
	}
}

func writeDeviceErr(w http.ResponseWriter, err error) {
	if err.Error() == "revoked" {
		problem.Write(w, http.StatusUnauthorized, "DEVICE_REVOKED", "this device was revoked")
		return
	}
	if _, ok := err.(errString); ok {
		problem.Write(w, http.StatusUnauthorized, "SESSION_REQUIRED", "session required")
		return
	}
	problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store unavailable")
}

func (s *Service) clientIP(r *http.Request) string {
	if s.TrustCF {
		if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); ip != "" {
			return ip
		}
	}
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
