package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spencercornish/booklab/internal/config"
)

func TestSecurityHeaders(t *testing.T) {
	for _, tc := range []struct {
		appURL   string
		wantHSTS bool
	}{
		{"https://book.example.com", true},
		{"http://localhost:8080", false},
	} {
		srv := New(&config.Config{AppURL: tc.appURL}, nil, nil, nil, nil, testLogger())
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/admin", nil))
		h := rec.Header()

		if csp := h.Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") || !strings.Contains(csp, "script-src 'self' https://js.stripe.com") {
			t.Errorf("%s: unexpected CSP %q", tc.appURL, csp)
		}
		if got := h.Get("X-Frame-Options"); got != "DENY" {
			t.Errorf("%s: X-Frame-Options = %q", tc.appURL, got)
		}
		if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options = %q", tc.appURL, got)
		}
		if got := h.Get("Permissions-Policy"); got != "camera=(), microphone=(), geolocation=()" {
			t.Errorf("%s: Permissions-Policy = %q", tc.appURL, got)
		}
		if got := h.Get("Strict-Transport-Security") != ""; got != tc.wantHSTS {
			t.Errorf("%s: HSTS present = %v, want %v", tc.appURL, got, tc.wantHSTS)
		}
	}
}
