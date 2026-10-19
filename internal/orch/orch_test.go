package orch

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"colorbook/internal/svcjwt"
)

func TestCloseWinnerIsIdempotent(t *testing.T) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	var hits int
	mux := http.NewServeMux()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	check := func(r *http.Request, aud string) {
		t.Helper()
		if _, err := svcjwt.Require(r.Header.Get("Authorization"), pub, aud, svcjwt.RoleOrchestrator, time.Now()); err != nil {
			t.Fatalf("%s: %v", aud, err)
		}
	}
	mux.HandleFunc("POST /internal/v1/rounds/{id}/tally", func(w http.ResponseWriter, r *http.Request) {
		check(r, svcjwt.AudVoting)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "winner", "word_id": "1", "text": "hello"})
	})
	mux.HandleFunc("POST /internal/v1/canvas/append", func(w http.ResponseWriter, r *http.Request) {
		check(r, svcjwt.AudCanvas)
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	o := &Orch{RDB: rdb, Proxy: srv.URL, Signer: priv, ID: "a"}
	ctx := context.Background()
	if err := o.Close(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if err := o.Close(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("append calls: %d", hits)
	}
}

func TestCloseSkipDoesNotAppend(t *testing.T) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/v1/rounds/{id}/tally", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "skip", "reason": "tie"})
	})
	mux.HandleFunc("POST /internal/v1/canvas/append", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("skip must not append")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	o := &Orch{RDB: rdb, Proxy: srv.URL, Signer: priv, ID: "a"}
	if err := o.Close(context.Background(), 101); err != nil {
		t.Fatal(err)
	}
}
