package script

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"colorbook/internal/keys"
)

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	s := miniredis.RunT(t)
	return redis.NewClient(&redis.Options{Addr: s.Addr()})
}

func TestSubmitExactIdentity(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	round := int64(10)
	k := []string{keys.Dedupe(round), keys.Submit(round, "a"), keys.Words(round), keys.Order(round), keys.Seq(round)}
	id1, created, status, err := RunSubmit(ctx, rdb, k, "hello", 60)
	if err != nil || !created || status != "NEW" {
		t.Fatalf("first: %s %v %s %v", id1, created, status, err)
	}
	k2 := []string{keys.Dedupe(round), keys.Submit(round, "b"), keys.Words(round), keys.Order(round), keys.Seq(round)}
	id2, created, status, err := RunSubmit(ctx, rdb, k2, "hello", 60)
	if err != nil || created || status != "JOIN" || id2 != id1 {
		t.Fatalf("join: %s %v %s %v", id2, created, status, err)
	}
	k3 := []string{keys.Dedupe(round), keys.Submit(round, "c"), keys.Words(round), keys.Order(round), keys.Seq(round)}
	id3, created, _, err := RunSubmit(ctx, rdb, k3, "heLLO", 60)
	if err != nil || !created || id3 == id1 {
		t.Fatalf("case must be distinct: %s %v %v", id3, created, err)
	}
	for _, text := range []string{"hello!", "h e l l o", "HELLO"} {
		kk := []string{keys.Dedupe(round), keys.Submit(round, text), keys.Words(round), keys.Order(round), keys.Seq(round)}
		id, created, _, err := RunSubmit(ctx, rdb, kk, text, 60)
		if err != nil || !created || id == id1 {
			t.Fatalf("%q must be its own row: %s %v %v", text, id, created, err)
		}
	}
	_, _, status, err = RunSubmit(ctx, rdb, k, "other", 60)
	if err != nil || status != "ALREADY" {
		t.Fatalf("second text: %s %v", status, err)
	}
}

func TestVoteSeqAndSwitch(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	round := int64(3)
	words := keys.Words(round)
	rdb.HSet(ctx, words, "1", "hello", "2", "heLLO")
	vote := func(did, word string, seq int) string {
		st, _, err := RunVote(ctx, rdb, []string{
			keys.Vote(round, did), keys.Tally(round), keys.Submit(round, did), words,
		}, word, seq, 60)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	rdb.Set(ctx, keys.Submit(round, "owner"), "1", 0)
	if vote("owner", "1", 1) != "OK" {
		t.Fatal("own word must be votable")
	}
	if vote("v", "1", 1) != "OK" {
		t.Fatal("first vote")
	}
	if vote("v", "1", 1) != "STALE" {
		t.Fatal("same seq must be stale")
	}
	if vote("v", "2", 2) != "OK" {
		t.Fatal("change")
	}
	if vote("v", "1", 2) != "STALE" {
		t.Fatal("delayed older seq")
	}
	n1, _ := rdb.HGet(ctx, keys.Tally(round), "1").Int()
	n2, _ := rdb.HGet(ctx, keys.Tally(round), "2").Int()
	if n1 != 1 || n2 != 1 {
		t.Fatalf("tally 1=%d 2=%d", n1, n2)
	}
}
