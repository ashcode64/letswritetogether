package voting

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/redis/go-redis/v9"

	"colorbook/internal/keys"
	"colorbook/internal/live"
	"colorbook/internal/problem"
	"colorbook/internal/script"
	"colorbook/internal/svcjwt"
)

type Service struct {
	RDB        *redis.Client
	GatewayPub ed25519.PublicKey
	OrchPub    ed25519.PublicKey
}

func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("PUT /v1/rounds/{id}/vote", s.put)
	mux.HandleFunc("POST /internal/v1/rounds/{id}/tally", s.tally)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

type putBody struct {
	WordID json.RawMessage `json:"word_id"`
	Seq    int             `json:"seq"`
}

func (s *Service) put(w http.ResponseWriter, r *http.Request) {
	did, err := svcjwt.Subject(r, s.GatewayPub, svcjwt.AudVoting, svcjwt.RoleGateway)
	if err != nil {
		problem.Write(w, http.StatusUnauthorized, "UNAUTHORIZED", "session required")
		return
	}
	round, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		problem.Write(w, http.StatusBadRequest, "BAD_WORD", "bad round")
		return
	}
	var body putBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Seq < 1 {
		problem.Write(w, http.StatusBadRequest, "BAD_WORD", "word_id and seq are required")
		return
	}
	word, ok := voteWord(body.WordID)
	if !ok {
		problem.Write(w, http.StatusBadRequest, "BAD_WORD", "word_id and seq are required")
		return
	}
	phase, err := live.ReadPhase(r.Context(), s.RDB)
	if err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "clock unavailable")
		return
	}
	if phase.RoundID != round {
		problem.Write(w, http.StatusConflict, "VOTING_CLOSED", "voting is closed for this minute")
		return
	}
	status, _, err := script.RunVote(r.Context(), s.RDB, []string{
		keys.Vote(round, did), keys.Tally(round), keys.Submit(round, did), keys.Words(round),
	}, word, body.Seq, keys.TTL)
	if err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store unavailable")
		return
	}
	switch status {
	case "MISSING":
		problem.Write(w, http.StatusBadRequest, "BAD_WORD", "unknown word")
	case "STALE":
		writeJSON(w, http.StatusOK, map[string]any{"code": "STALE_SEQ", "word_id": word})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"word_id": word, "seq": body.Seq})
	}
}

// voteWord accepts a word id, or null to remove the vote. A missing field is refused.
func voteWord(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		if len(raw) == 0 {
			return "", false
		}
		return "", true
	}
	var word string
	if err := json.Unmarshal(raw, &word); err != nil {
		return "", false
	}
	return word, true
}

func (s *Service) tally(w http.ResponseWriter, r *http.Request) {
	if _, err := svcjwt.Subject(r, s.OrchPub, svcjwt.AudVoting, svcjwt.RoleOrchestrator); err != nil {
		problem.Write(w, http.StatusUnauthorized, "UNAUTHORIZED", "orchestrator token required")
		return
	}
	round, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		problem.Write(w, http.StatusBadRequest, "BAD_WORD", "bad round")
		return
	}
	counts, err := s.RDB.HGetAll(r.Context(), keys.Tally(round)).Result()
	if err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store unavailable")
		return
	}
	best := 0
	var winners []string
	for id, raw := range counts {
		n, _ := strconv.Atoi(raw)
		if n <= 0 {
			continue
		}
		if n > best {
			best = n
			winners = []string{id}
		} else if n == best {
			winners = append(winners, id)
		}
	}
	if best == 0 {
		reason := "zero"
		n, err := s.RDB.HLen(r.Context(), keys.Words(round)).Result()
		if err != nil {
			problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store unavailable")
			return
		}
		if n == 0 {
			reason = "empty"
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "skip", "reason": reason})
		return
	}
	winner, err := s.firstSubmitted(r.Context(), round, winners)
	if err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store unavailable")
		return
	}
	if winner == "" {
		writeJSON(w, http.StatusOK, map[string]any{"status": "skip", "reason": "tie"})
		return
	}
	text, err := s.RDB.HGet(r.Context(), keys.Words(round), winner).Result()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "skip", "reason": "empty"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "winner", "word_id": winner, "text": text})
}

// firstSubmitted returns the tied word that appears first in the arrival list.
// A single leader does not need that list. A tie with no arrival record is not guessed.
func (s *Service) firstSubmitted(ctx context.Context, round int64, winners []string) (string, error) {
	if len(winners) == 1 {
		return winners[0], nil
	}
	order, err := s.RDB.LRange(ctx, keys.Order(round), 0, -1).Result()
	if err != nil {
		return "", err
	}
	want := make(map[string]struct{}, len(winners))
	for _, id := range winners {
		want[id] = struct{}{}
	}
	for _, id := range order {
		if _, ok := want[id]; ok {
			return id, nil
		}
	}
	return "", nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
