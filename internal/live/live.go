package live

import (
	"context"
	"encoding/json"
	"os"

	"github.com/redis/go-redis/v9"

	"colorbook/internal/clock"
	"colorbook/internal/keys"
)

func Open() *redis.Client {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	return redis.NewClient(&redis.Options{Addr: addr})
}

func Time(ctx context.Context, rdb *redis.Client) (clock.Snapshot, error) {
	sec, usec, err := timeParts(ctx, rdb)
	if err != nil {
		return clock.Snapshot{}, err
	}
	return clock.From(sec, usec), nil
}

func timeParts(ctx context.Context, rdb *redis.Client) (int64, int64, error) {
	res, err := rdb.Time(ctx).Result()
	if err != nil {
		return 0, 0, err
	}
	return res.Unix(), int64(res.Nanosecond() / 1000), nil
}

func WritePhase(ctx context.Context, rdb *redis.Client, s clock.Snapshot) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return rdb.Set(ctx, keys.PhaseKey, b, 0).Err()
}

func ReadPhase(ctx context.Context, rdb *redis.Client) (clock.Snapshot, error) {
	raw, err := rdb.Get(ctx, keys.PhaseKey).Bytes()
	if err == redis.Nil {
		return Time(ctx, rdb)
	}
	if err != nil {
		return clock.Snapshot{}, err
	}
	var s clock.Snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return clock.Snapshot{}, err
	}
	return s, nil
}

func Publish(ctx context.Context, rdb *redis.Client, event string, fields map[string]any) error {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["type"] = event
	b, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return rdb.Publish(ctx, keys.Channel, b).Err()
}
