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

type contextKey string

const adminUsernameKey contextKey = "adminUsername"

func adminUsernameFromContext(ctx context.Context) (string, bool) {
	username, ok := ctx.Value(adminUsernameKey).(string)
	return username, ok
}

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

// setSessionCookie writes the admin session cookie with the standard attributes.
func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     config.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(config.SessionDuration.Seconds()),
	})
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("booklab_session")
		if err != nil {
			s.logger.Warn("admin_auth_missing_session", "path", r.URL.Path)
			s.writeError(w, r, http.StatusUnauthorized, "authentication required", err)
			return
		}
		sess, err := s.queries.GetAdminSession(r.Context(), cookie.Value)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				s.logger.Warn("admin_invalid_session", "path", r.URL.Path)
				s.writeError(w, r, http.StatusUnauthorized, "invalid or expired session", err)
				return
			}
			s.writeError(w, r, http.StatusInternalServerError, "session check failed", err)
			return
		}
		if _, err := s.queries.GetAdminByUsername(r.Context(), sess.Username); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				s.logger.Warn("admin_invalid_session", "path", r.URL.Path)
				s.writeError(w, r, http.StatusUnauthorized, "invalid or expired session", err)
				return
			}
			s.writeError(w, r, http.StatusInternalServerError, "session check failed", err)
			return
		}
		ctx := context.WithValue(r.Context(), adminUsernameKey, sess.Username)
		next.ServeHTTP(w, r.WithContext(ctx))
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
				s.writeError(w, r, http.StatusInternalServerError, "csrf init failed", err)
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
				s.logger.Warn("admin_csrf_mismatch", "path", r.URL.Path, "method", r.Method)
				s.writeError(w, r, http.StatusForbidden, "invalid csrf token", nil)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
