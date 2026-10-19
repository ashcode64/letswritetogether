package archive

import (
	"crypto/ed25519"
	"net/http"

	"colorbook/internal/pgstore"
	"colorbook/internal/problem"
	"colorbook/internal/svcjwt"
)

type Service struct {
	Store   *pgstore.Store
	OrchPub ed25519.PublicKey
}

func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /internal/v1/days/{day}/seal", s.seal)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func (s *Service) seal(w http.ResponseWriter, r *http.Request) {
	if _, err := svcjwt.Subject(r, s.OrchPub, svcjwt.AudArchive, svcjwt.RoleOrchestrator); err != nil {
		problem.Write(w, http.StatusUnauthorized, "UNAUTHORIZED", "orchestrator token required")
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
