package orch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestCloseWinnerIsIdempotent(t *testing.T) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	var hits int
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/v1/rounds/{id}/tally", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") != "tok" {
			t.Fatal("missing token")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "winner", "word_id": "1", "text": "hello"})
	})
	mux.HandleFunc("POST /internal/v1/canvas/append", func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	o := &Orch{RDB: rdb, Proxy: srv.URL, Token: "tok", ID: "a"}
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
	o := &Orch{RDB: rdb, Proxy: srv.URL, Token: "tok", ID: "a"}
	if err := o.Close(context.Background(), 101); err != nil {
		t.Fatal(err)
	}
}
