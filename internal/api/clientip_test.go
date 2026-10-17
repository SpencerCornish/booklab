package api

import (
	"net/http/httptest"
	"testing"

	"github.com/spencercornish/booklab/internal/config"
)

func TestClientIP(t *testing.T) {
	srv := New(&config.Config{TrustedProxies: "10.0.0.0/8, 127.0.0.1"}, nil, nil, nil, nil, testLogger())

	cases := []struct {
		name   string
		remote string
		xff    []string
		want   string
	}{
		{"untrusted peer ignores header", "203.0.113.5:1234", []string{"198.51.100.7"}, "203.0.113.5"},
		{"trusted peer uses forwarded client", "10.0.0.2:1234", []string{"198.51.100.7"}, "198.51.100.7"},
		{"client-written entries are skipped", "10.0.0.2:1234", []string{"6.6.6.6, 198.51.100.7"}, "198.51.100.7"},
		{"chained trusted proxies", "127.0.0.1:1234", []string{"198.51.100.7, 10.1.2.3"}, "198.51.100.7"},
		{"multiple header lines", "10.0.0.2:1234", []string{"6.6.6.6", "198.51.100.7"}, "198.51.100.7"},
		{"no header falls back to peer", "10.0.0.2:1234", nil, "10.0.0.2"},
		{"garbage header falls back to peer", "10.0.0.2:1234", []string{"not-an-ip"}, "10.0.0.2"},
		{"all hops trusted uses leftmost", "10.0.0.2:1234", []string{"10.9.9.9, 10.1.1.1"}, "10.9.9.9"},
		{"ipv4-mapped trusted peer", "[::ffff:10.0.0.2]:1234", []string{"198.51.100.7"}, "198.51.100.7"},
		{"ipv6 untrusted peer", "[2001:db8::1]:1234", []string{"198.51.100.7"}, "2001:db8::1"},
	}
	for _, tc := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.remote
		for _, v := range tc.xff {
			r.Header.Add("X-Forwarded-For", v)
		}
		if got := srv.clientIP(r); got != tc.want {
			t.Errorf("%s: clientIP = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestClientIP_noTrustedProxies(t *testing.T) {
	srv := New(&config.Config{}, nil, nil, nil, nil, testLogger())
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	r.Header.Set("X-Real-IP", "198.51.100.8")
	r.Header.Set("True-Client-IP", "198.51.100.9")
	if got := srv.clientIP(r); got != "127.0.0.1" {
		t.Fatalf("clientIP = %q, want 127.0.0.1", got)
	}
}
