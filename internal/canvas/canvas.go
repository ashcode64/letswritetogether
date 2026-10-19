package canvas

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"colorbook/internal/live"
	"colorbook/internal/pgstore"
	"colorbook/internal/problem"
	"colorbook/internal/svcjwt"
)

type Service struct {
	Store   *pgstore.Store
	RDB     *redis.Client
	OrchPub ed25519.PublicKey
}

func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/canvas/today", s.today)
	mux.HandleFunc("GET /v1/canvas/days", s.days)
	mux.HandleFunc("GET /v1/canvas/{day}", s.byDay)
	mux.HandleFunc("POST /internal/v1/canvas/append", s.append)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func (s *Service) today(w http.ResponseWriter, r *http.Request) {
	s.writeDay(w, r, s.utcDay(r), true)
}

func (s *Service) days(w http.ResponseWriter, r *http.Request) {
	today := s.utcDay(r)
	t, err := time.Parse(time.DateOnly, today)
	if err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "clock unavailable")
		return
	}
	oldest := t.AddDate(0, 0, -30).Format(time.DateOnly)
	days, err := s.Store.DaysWithWords(r.Context(), oldest)
	if err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"days": days})
}

func (s *Service) byDay(w http.ResponseWriter, r *http.Request) {
	day := r.PathValue("day")
	if _, err := time.Parse(time.DateOnly, day); err != nil {
		problem.Write(w, http.StatusBadRequest, "BAD_WORD", "day must be YYYY-MM-DD")
		return
	}
	s.writeDay(w, r, day, day == s.utcDay(r))
}

// utcDay follows Redis TIME, the same clock the orchestrator uses for a winner's day.
func (s *Service) utcDay(r *http.Request) string {
	if s.RDB != nil {
		if snap, err := live.Time(r.Context(), s.RDB); err == nil {
			return time.Unix(snap.ServerTime, 0).UTC().Format(time.DateOnly)
		}
	}
	return time.Now().UTC().Format(time.DateOnly)
}

func (s *Service) writeDay(w http.ResponseWriter, r *http.Request, day string, today bool) {
	es, err := s.Store.Get(r.Context(), day)
	if err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store unavailable")
		return
	}
	if !today && !es.Found {
		problem.Write(w, http.StatusNotFound, "NOT_FOUND", "no essay for that day")
		return
	}
	if es.Sealed {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	writeJSON(w, http.StatusOK, es)
}

type appendBody struct {
	Day         string `json:"day"`
	RoundNumber int64  `json:"round_number"`
	Word        string `json:"word"`
}

func (s *Service) append(w http.ResponseWriter, r *http.Request) {
	if _, err := svcjwt.Subject(r, s.OrchPub, svcjwt.AudCanvas, svcjwt.RoleOrchestrator); err != nil {
		problem.Write(w, http.StatusUnauthorized, "UNAUTHORIZED", "orchestrator token required")
		return
	}
	var body appendBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Word == "" || body.RoundNumber == 0 {
		problem.Write(w, http.StatusBadRequest, "BAD_WORD", "day, round_number, and word are required")
		return
	}
	if err := s.Store.Append(r.Context(), body.Day, body.RoundNumber, body.Word); err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
