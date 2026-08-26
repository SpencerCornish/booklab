package api

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testWebFS(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"index.html":              {Data: []byte("<html>app</html>")},
		"assets/app.js":           {Data: []byte("console.log('hi')")},
		"favicon.svg":             {Data: []byte("<svg/>")},
	}
}

func TestIsProbePath(t *testing.T) {
	probes := []string{
		"/.env",
		"/.env.local",
		"/.git/config",
		"/phpinfo.php",
		"/wp-config.php",
		"/wp-admin/install.php",
		"/vendor/phpunit/phpunit/src/Util/PHP/eval-stdin.php",
		"/actuator/health",
		"/shell.php",
		"/backup.sql",
	}
	for _, p := range probes {
		if !isProbePath(p) {
			t.Errorf("isProbePath(%q) = false, want true", p)
		}
	}

	legit := []string{
		"/",
		"/admin",
		"/admin/login",
		"/booking/abc123",
		"/cancel/abc123",
		"/terms",
		"/privacy",
		"/assets/app.js",
		"/api/bookings",
	}
	for _, p := range legit {
		if isProbePath(p) {
			t.Errorf("isProbePath(%q) = true, want false", p)
		}
	}
}

func TestIsSPARoute(t *testing.T) {
	routes := []string{"/", "/admin", "/admin/bookings", "/booking/tok", "/cancel/tok", "/terms", "/privacy"}
	for _, p := range routes {
		if !isSPARoute(p) {
			t.Errorf("isSPARoute(%q) = false, want true", p)
		}
	}

	nonRoutes := []string{"/random", "/foo", "/robots.txt", "/.env"}
	for _, p := range nonRoutes {
		if isSPARoute(p) {
			t.Errorf("isSPARoute(%q) = true, want false", p)
		}
	}
}

func TestHandleSPA(t *testing.T) {
	srv := New(testConfig(), nil, nil, nil, testWebFS(t), testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	tests := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{"/", http.StatusOK, "<html>app</html>"},
		{"/admin", http.StatusOK, "<html>app</html>"},
		{"/booking/abc", http.StatusOK, "<html>app</html>"},
		{"/assets/app.js", http.StatusOK, "console.log"},
		{"/favicon.svg", http.StatusOK, "<svg/>"},
		{"/.env", http.StatusNotFound, ""},
		{"/phpinfo.php", http.StatusNotFound, ""},
		{"/random-scanner-path", http.StatusNotFound, ""},
		{"/robots.txt", http.StatusNotFound, ""},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			resp, err := http.Get(ts.URL + tc.path)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			if tc.wantBody != "" {
				body, _ := io.ReadAll(resp.Body)
				if !strings.Contains(string(body), tc.wantBody) {
					t.Fatalf("body %q does not contain %q", body, tc.wantBody)
				}
			}
		})
	}
}
