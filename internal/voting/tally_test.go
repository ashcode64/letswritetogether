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
	rdb.RPush(ctx, keys.Order(3), "1", "2")
	tied := call("3")
	if tied["status"] != "winner" || tied["text"] != "hello" || tied["word_id"] != "1" {
		t.Fatalf("earliest tied word: %v", tied)
	}
	rdb.HSet(ctx, keys.Words(5), "10", "later", "2", "earlier")
	rdb.HSet(ctx, keys.Tally(5), "10", "4", "2", "4")
	rdb.RPush(ctx, keys.Order(5), "10", "2")
	order := call("5")
	if order["text"] != "later" || order["word_id"] != "10" {
		t.Fatalf("arrival order: %v", order)
	}
	rdb.HSet(ctx, keys.Words(6), "1", "is", "2", "a")
	rdb.HSet(ctx, keys.Tally(6), "1", "1", "2", "1")
	rdb.RPush(ctx, keys.Order(6), "1", "2")
	words := call("6")
	if words["text"] != "is" || words["word_id"] != "1" {
		t.Fatalf("is before a: %v", words)
	}
	rdb.HSet(ctx, keys.Words(4), "1", "hello!")
	rdb.HSet(ctx, keys.Tally(4), "1", "3")
	got := call("4")
	if got["status"] != "winner" || got["text"] != "hello!" {
		t.Fatalf("%v", got)
	}
}
