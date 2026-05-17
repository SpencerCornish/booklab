package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/spencercornish/booklab/internal/config"
)

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
