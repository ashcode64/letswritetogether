package voting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"colorbook/internal/keys"
)

func TestTallySkipAndWinner(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := &Service{RDB: rdb, Token: "t"}
	mux := http.NewServeMux()
	svc.Register(mux)

	call := func(round string) map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/internal/v1/rounds/"+round+"/tally", nil)
		req.Header.Set("X-Internal-Token", "t")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("round %s: %d %s", round, rec.Code, rec.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}

	if call("1")["reason"] != "empty" {
		t.Fatal(call("1"))
	}
	rdb.HSet(ctx, keys.Words(2), "1", "hello")
	if call("2")["reason"] != "zero" {
		t.Fatal(call("2"))
	}
	rdb.HSet(ctx, keys.Words(3), "1", "hello", "2", "heLLO")
	rdb.HSet(ctx, keys.Tally(3), "1", "2", "2", "2")
	tied := call("3")
	if tied["status"] != "winner" || tied["text"] != "hello" || tied["word_id"] != "1" {
		t.Fatalf("earliest tied word: %v", tied)
	}
	rdb.HSet(ctx, keys.Words(5), "10", "later", "2", "earlier")
	rdb.HSet(ctx, keys.Tally(5), "10", "4", "2", "4")
	order := call("5")
	if order["text"] != "earlier" || order["word_id"] != "2" {
		t.Fatalf("numeric id order: %v", order)
	}
	rdb.HSet(ctx, keys.Words(4), "1", "hello!")
	rdb.HSet(ctx, keys.Tally(4), "1", "3")
	got := call("4")
	if got["status"] != "winner" || got["text"] != "hello!" {
		t.Fatalf("%v", got)
	}
}
