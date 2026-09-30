package archive

import (
	"net/http"

	"colorbook/internal/pgstore"
	"colorbook/internal/problem"
)

type Service struct {
	Store *pgstore.Store
	Token string
}

func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /internal/v1/days/{day}/seal", s.seal)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func (s *Service) seal(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Internal-Token") != s.Token || s.Token == "" {
		problem.Write(w, http.StatusUnauthorized, "UNAUTHORIZED", "internal token required")
		return
	}
	day := r.PathValue("day")
	if err := s.Store.Seal(r.Context(), day); err != nil {
		problem.Write(w, http.StatusBadRequest, "BAD_WORD", "could not seal that day")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}
