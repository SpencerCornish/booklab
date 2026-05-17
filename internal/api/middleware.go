package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// sessionToken is a simple HMAC-signed token: "username:expiry:sig"
func newSessionToken(username, secret string) string {
	expiry := time.Now().Add(24 * time.Hour).Unix()
	payload := fmt.Sprintf("%s:%d", username, expiry)
	sig := sign(payload, secret)
	return fmt.Sprintf("%s:%s", payload, sig)
}

func validateSessionToken(token, secret string) (string, bool) {
	parts := strings.Split(token, ":")
	if len(parts) != 3 {
		return "", false
	}
	username := parts[0]
	expiryStr := parts[1]
	sig := parts[2]

	payload := fmt.Sprintf("%s:%s", username, expiryStr)
	expectedSig := sign(payload, secret)
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return "", false
	}

	var expiry int64
	if _, err := fmt.Sscanf(expiryStr, "%d", &expiry); err != nil {
		return "", false
	}
	if time.Now().Unix() > expiry {
		return "", false
	}
	return username, true
}

func sign(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("booklab_session")
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		username, ok := validateSessionToken(cookie.Value, s.cfg.SessionSecret)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid or expired session")
			return
		}
		_ = username
		next.ServeHTTP(w, r)
	})
}
