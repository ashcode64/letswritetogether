package problem

import (
	"encoding/json"
	"net/http"
)

// Write sends an application/problem+json body with a stable code.
func Write(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  code,
		"status": status,
		"code":   code,
		"detail": detail,
	})
}
