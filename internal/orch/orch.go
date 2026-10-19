package orch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"crypto/ed25519"

	"colorbook/internal/clock"
	"colorbook/internal/keys"
	"colorbook/internal/live"
	"colorbook/internal/svcjwt"
)

type Orch struct {
	RDB      *redis.Client
	Proxy    string
	Signer   ed25519.PrivateKey
	ID       string
	HTTP     *http.Client
	ready    bool
	last     int64
	sealed   string
	pubRound int64
	pubPhase string
}

func (o *Orch) Hold(ctx context.Context) (bool, error) {
	ok, err := o.RDB.SetNX(ctx, keys.LeaseKey, o.ID, 3*time.Second).Result()
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	cur, err := o.RDB.Get(ctx, keys.LeaseKey).Result()
	if err != nil {
		return false, nil
	}
	if cur != o.ID {
		return false, nil
	}
	return true, o.RDB.PExpire(ctx, keys.LeaseKey, 3*time.Second).Err()
}

// Tick writes the phase and, on a new minute, closes the previous one.
// It does nothing when this process does not hold the lease.
func (o *Orch) Tick(ctx context.Context) error {
	hold, err := o.Hold(ctx)
	if err != nil || !hold {
		return err
	}
	snap, err := live.Time(ctx, o.RDB)
	if err != nil {
		return err
	}
	if err := live.WritePhase(ctx, o.RDB, snap); err != nil {
		return err
	}
	if snap.RoundID != o.pubRound || snap.Phase != o.pubPhase {
		if err := live.Publish(ctx, o.RDB, "round.phase", phaseFields(snap)); err != nil {
			return err
		}
		o.pubRound, o.pubPhase = snap.RoundID, snap.Phase
	}
	if !o.ready {
		o.ready = true
		o.last = snap.RoundID
		if snap.RoundID > 0 {
			if err := o.Close(ctx, snap.RoundID-1); err != nil {
				o.last = snap.RoundID - 1
				return err
			}
		}
		return o.maybeSeal(ctx, snap.ServerTime)
	}
	for r := o.last; r < snap.RoundID; r++ {
		if err := o.Close(ctx, r); err != nil {
			return err
		}
	}
	o.last = snap.RoundID
	return o.maybeSeal(ctx, snap.ServerTime)
}

func phaseFields(s clock.Snapshot) map[string]any {
	return map[string]any{
		"round_id": s.RoundID, "phase": s.Phase,
		"server_time": s.ServerTime, "server_time_ms": s.ServerTimeMS,
		"submit_ms_left": s.SubmitMSLeft, "vote_ms_left": s.VoteMSLeft,
	}
}

type tally struct {
	Status string `json:"status"`
	WordID string `json:"word_id"`
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

// Close asks voting for the tally and appends a unique winner.
// A stored outcome makes a retry a no-op, so a skip cannot become a blank slot later.
func (o *Orch) Close(ctx context.Context, round int64) error {
	key := keys.Closed(round)
	if n, err := o.RDB.Exists(ctx, key).Result(); err != nil {
		return err
	} else if n > 0 {
		return nil
	}
	var t tally
	if err := o.post(ctx, fmt.Sprintf("%s/internal/v1/rounds/%d/tally", o.Proxy, round), nil, &t, svcjwt.AudVoting); err != nil {
		return err
	}
	day := time.Unix(round*60, 0).UTC().Format(time.DateOnly)
	if t.Status == "winner" && t.Text != "" {
		body := map[string]any{"day": day, "round_number": round, "word": t.Text}
		if err := o.post(ctx, o.Proxy+"/internal/v1/canvas/append", body, nil, svcjwt.AudCanvas); err != nil {
			return err
		}
		if err := o.RDB.Set(ctx, key, "win", 48*time.Hour).Err(); err != nil {
			return err
		}
		return live.Publish(ctx, o.RDB, "round.winner", map[string]any{
			"round_id": round, "word_id": t.WordID, "text": t.Text, "day": day,
		})
	}
	if err := o.RDB.Set(ctx, key, "skip", 48*time.Hour).Err(); err != nil {
		return err
	}
	return live.Publish(ctx, o.RDB, "round.skipped", map[string]any{
		"round_id": round, "reason": t.Reason,
	})
}

func (o *Orch) maybeSeal(ctx context.Context, sec int64) error {
	today := time.Unix(sec, 0).UTC().Format(time.DateOnly)
	if o.sealed == today {
		return nil
	}
	yday := time.Unix(sec, 0).UTC().AddDate(0, 0, -1).Format(time.DateOnly)
	if err := o.post(ctx, fmt.Sprintf("%s/internal/v1/days/%s/seal", o.Proxy, yday), nil, nil, svcjwt.AudArchive); err != nil {
		return err
	}
	o.sealed = today
	return nil
}

func (o *Orch) post(ctx context.Context, url string, body any, out any, aud string) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, r)
	if err != nil {
		return err
	}
	now := time.Now()
	tok, err := svcjwt.Sign(o.Signer, svcjwt.Claims{
		Aud: aud, Role: svcjwt.RoleOrchestrator, Iat: now.Unix(), Exp: now.Add(30 * time.Second).Unix(),
	})
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := o.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("POST %s: %s", url, res.Status)
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}
