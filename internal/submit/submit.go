package submit

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"

	"crypto/ed25519"

	"colorbook/internal/filter"
	"colorbook/internal/keys"
	"colorbook/internal/live"
	"colorbook/internal/problem"
	"colorbook/internal/script"
	"colorbook/internal/svcjwt"
	"colorbook/internal/words"
)

type Service struct {
	RDB        *redis.Client
	Filter     *filter.Filter
	GatewayPub ed25519.PublicKey
}

func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/rounds/{id}/words", s.post)
	mux.HandleFunc("GET /v1/rounds/{id}/words", s.list)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

type postBody struct {
	Text string `json:"text"`
	Seq  int    `json:"seq"`
}

func (s *Service) post(w http.ResponseWriter, r *http.Request) {
	did, ok := s.device(w, r)
	if !ok {
		return
	}
	round, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		problem.Write(w, http.StatusBadRequest, "BAD_WORD", "bad round")
		return
	}
	var body postBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problem.Write(w, http.StatusBadRequest, "BAD_WORD", "malformed body")
		return
	}
	text, ok := words.Validate(body.Text)
	if !ok {
		problem.Write(w, http.StatusUnprocessableEntity, "BAD_WORD", "use letters, spaces, and essay punctuation")
		return
	}
	if s.Filter.Reject(text) {
		problem.Write(w, http.StatusUnprocessableEntity, "WORD_REJECTED", "that text is not allowed")
		return
	}
	phase, err := live.ReadPhase(r.Context(), s.RDB)
	if err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "clock unavailable")
		return
	}
	if phase.RoundID != round || phase.Phase != "submit" {
		problem.Write(w, http.StatusConflict, "SUBMISSION_CLOSED", "submissions are closed for this minute")
		return
	}
	id, created, status, err := script.RunSubmit(r.Context(), s.RDB, []string{
		keys.Dedupe(round), keys.Submit(round, did), keys.Words(round), keys.Order(round), keys.Seq(round),
	}, text, keys.TTL)
	if err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store unavailable")
		return
	}
	if status == "ALREADY" {
		problem.Write(w, http.StatusConflict, "ALREADY_SUBMITTED", "you already submitted this minute")
		return
	}
	if body.Seq >= 1 {
		_, _, _ = script.RunVote(r.Context(), s.RDB, []string{
			keys.Vote(round, did), keys.Tally(round), keys.Submit(round, did), keys.Words(round),
		}, id, body.Seq, keys.TTL)
	}
	if created {
		_ = live.Publish(r.Context(), s.RDB, "word.approved", map[string]any{
			"round_id": round, "word_id": id, "text": text,
		})
	}
	code := http.StatusOK
	if created {
		code = http.StatusCreated
	}
	writeJSON(w, code, map[string]any{"word_id": id, "text": text, "created": created})
}

func (s *Service) device(w http.ResponseWriter, r *http.Request) (string, bool) {
	did, err := svcjwt.Subject(r, s.GatewayPub, svcjwt.AudSubmission, svcjwt.RoleGateway)
	if err != nil {
		problem.Write(w, http.StatusUnauthorized, "UNAUTHORIZED", "session required")
		return "", false
	}
	return did, true
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.device(w, r); !ok {
		return
	}
	round, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		problem.Write(w, http.StatusBadRequest, "BAD_WORD", "bad round")
		return
	}
	ids, err := s.RDB.LRange(r.Context(), keys.Order(round), 0, -1).Result()
	if err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store unavailable")
		return
	}
	q := r.URL.Query().Get("q")
	type row struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	out := []row{}
	for _, id := range ids {
		text, err := s.RDB.HGet(r.Context(), keys.Words(round), id).Result()
		if err != nil {
			continue
		}
		if q != "" && !strings.Contains(text, q) {
			continue
		}
		out = append(out, row{ID: id, Text: text})
	}
	writeJSON(w, http.StatusOK, map[string]any{"words": out})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
