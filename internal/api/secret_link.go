package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"

	"github.com/spencercornish/booklab/internal/db"
)

// secretLinkURLPath is the SPA route that serves the booking page in
// secret-link mode. Tokens travel in header/query parameters only (never the
// path) so request logs keep clear of them; this prefix routes the deep link.
const secretLinkURLPath = "/book/"

// generateSecretLinkToken returns 32 random bytes hex-encoded (64 chars), the
// same entropy as the admin session IDs.
func generateSecretLinkToken() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// secretTokenFromRequest extracts the secret-link token from the
// X-Secret-Token header, falling back to the secret_token query parameter on
// GETs. Tokens stay out of the URL path so access logs never record them.
func secretTokenFromRequest(r *http.Request) string {
	if t := r.Header.Get("X-Secret-Token"); t != "" {
		return t
	}
	return r.URL.Query().Get("secret_token")
}

// secretTokenFromBooking matches the create-booking payload: the token may ride
// in the X-Secret-Token header or the JSON body (the SPA client cannot set
// arbitrary headers through every fetch wrapper, and the body is not logged).
func secretTokenFromBooking(r *http.Request, bodyToken string) string {
	if t := r.Header.Get("X-Secret-Token"); t != "" {
		return t
	}
	return bodyToken
}

// validateSecretToken reports whether token is a live secret link: the feature
// is enabled, a token has been issued, and the comparison matches (constant
// time so probes cannot time-difference their way to the real token).
func validateSecretToken(settings *db.Settings, token string) bool {
	if !settings.SecretLinkEnabled || settings.SecretLinkToken == nil || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(*settings.SecretLinkToken), []byte(token)) == 1
}

// handleSecretLinkVerify answers whether a token is a live secret link.
// Both outcomes return 200 with a `valid` boolean: a uniform shape keeps the
// SPA contract trivial and gives token probes nothing to distinguish beyond
// what the body already says.
func (s *Server) handleSecretLinkVerify(w http.ResponseWriter, r *http.Request) {
	settings, err := s.queries.GetSettings(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load settings", err)
		return
	}

	token := r.URL.Query().Get("token")
	if !validateSecretToken(settings, token) {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":         true,
		"resource_name": settings.ResourceName,
		"any_hours":     settings.SecretLinkAnyHours,
	})
}

// handleAdminRotateSecretLink issues a fresh token, revoking the previous link
// immediately. Rotation is admin-only; the token never round-trips through the
// public settings.
func (s *Server) handleAdminRotateSecretLink(w http.ResponseWriter, r *http.Request) {
	token, err := generateSecretLinkToken()
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to generate token", err)
		return
	}
	if _, err := s.queries.SetSecretLinkToken(r.Context(), token); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to save token", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"url":   s.cfg.AppURL + secretLinkURLPath + token,
	})
}
