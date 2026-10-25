package api

import (
	"net/http"
)

// handleHealth is an unauthenticated liveness/readiness probe used by test
// harnesses, Compose healthchecks, and uptime monitors. It deliberately does
// not touch the database: the goal is a fast, dependency-free answer.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
