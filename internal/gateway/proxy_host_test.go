package gateway

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestProxyDirectorSetsProxyHost(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	var gotHost string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(up.Close)

	s := New(rdb, "secret", up.URL, "colorbook-internal", false, priv, testTurnstile, "site", 200)
	req := httptest.NewRequest(http.MethodGet, "/v1/canvas/today", nil)
	req.Host = "localhost"
	rec := httptest.NewRecorder()
	s.public("read", 120)(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if gotHost != "colorbook-internal" {
		t.Fatalf("upstream Host = %q, want colorbook-internal", gotHost)
	}
}
