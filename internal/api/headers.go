package api

import (
	"net/http"
	"strings"
)

// contentSecurityPolicy allows the SPA's own assets, Google Fonts (web/index.html) and the
// Stripe.js origins Stripe documents for Elements and 3D Secure. style-src needs
// 'unsafe-inline' because the editor and calendar libraries inject <style> elements.
var contentSecurityPolicy = strings.Join([]string{
	"default-src 'self'",
	"script-src 'self' https://js.stripe.com https://*.js.stripe.com",
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
	"font-src 'self' https://fonts.gstatic.com",
	"img-src 'self' data: https://*.stripe.com",
	"connect-src 'self' https://api.stripe.com",
	"frame-src https://js.stripe.com https://*.js.stripe.com https://hooks.stripe.com",
	"object-src 'none'",
	"base-uri 'self'",
	"form-action 'self'",
	"frame-ancestors 'none'",
}, "; ")

// securityHeaders sets browser hardening headers on every response. HSTS is only sent
// when APP_URL is https, so plain-http local development is not pinned to TLS.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	hsts := strings.HasPrefix(strings.ToLower(strings.TrimSpace(s.cfg.AppURL)), "https://")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if hsts {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}
