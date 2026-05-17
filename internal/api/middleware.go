package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/spencercornish/booklab/internal/config"
)

const csrfCookieName = "booklab_csrf"

func (s *Server) newAdminSession(ctx context.Context, username string) (sessionID string, err error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf[:])
	expiresAt := time.Now().Add(config.SessionDuration)
	if err := s.queries.CreateAdminSession(ctx, id, username, expiresAt); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("booklab_session")
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		sess, err := s.queries.GetAdminSession(r.Context(), cookie.Value)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusUnauthorized, "invalid or expired session")
				return
			}
			writeError(w, http.StatusInternalServerError, "session check failed")
			return
		}
		if _, err := s.queries.GetAdminByUsername(r.Context(), sess.Username); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusUnauthorized, "invalid or expired session")
				return
			}
			writeError(w, http.StatusInternalServerError, "session check failed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// csrfProtect enforces a double-submit cookie for admin mutating requests.
// The booklab_csrf cookie is readable by JS so the SPA can mirror it in X-CSRF-Token.
func (s *Server) csrfProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(csrfCookieName)
		token := ""
		if err == nil && cookie != nil && cookie.Value != "" {
			token = cookie.Value
		} else {
			var buf [32]byte
			if _, err := rand.Read(buf[:]); err != nil {
				writeError(w, http.StatusInternalServerError, "csrf init failed")
				return
			}
			token = hex.EncodeToString(buf[:])
			http.SetCookie(w, &http.Cookie{
				Name:     csrfCookieName,
				Value:    token,
				Path:     "/",
				MaxAge:   int(config.SessionDuration.Seconds()),
				HttpOnly: false,
				Secure:   true,
				SameSite: http.SameSiteStrictMode,
			})
		}

		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			hdr := r.Header.Get("X-CSRF-Token")
			if subtle.ConstantTimeCompare([]byte(hdr), []byte(token)) != 1 {
				writeError(w, http.StatusForbidden, "invalid csrf token")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
