package orch

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestPostSetsProxyHost(t *testing.T) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	var gotHost string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/v1/rounds/{id}/tally", func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "skip", "reason": "empty"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	o := &Orch{
		RDB: rdb, Proxy: srv.URL, ProxyHost: "sharedpage-internal",
		Signer: priv, ID: "a",
	}
	if err := o.Close(context.Background(), 200); err != nil {
		t.Fatal(err)
	}
	if gotHost != "sharedpage-internal" {
		t.Fatalf("Host = %q, want sharedpage-internal", gotHost)
	}
}
