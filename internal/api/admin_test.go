package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/spencercornish/booklab/internal/config"
	emailsvc "github.com/spencercornish/booklab/internal/email"
)

func testConfig() *config.Config {
	return &config.Config{
		AppURL:             "http://127.0.0.1:8080",
		CORSAllowedOrigins: "https://trusted.example",
		Port:               8080,
	}
}

func TestAdminChargeBooking_idempotentSecondRequest(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	ms := &mockStripe{}
	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	srv := New(testConfig(), q, ms, emailSvc, nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateAdminUser(ctx, "admin", string(hash)); err != nil {
		t.Fatal(err)
	}

	// Completed booking eligible for charge
	start := time.Date(2031, 3, 1, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	b, err := q.CreateBooking(ctx, "Pat", "pat@example.com", start, end, "si_x")
	if err != nil {
		t.Fatal(err)
	}
	pm := "pm_test_123"
	if _, err := q.UpdateBookingPaymentMethod(ctx, b.ID, pm); err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateBookingStatus(ctx, b.ID, "completed"); err != nil {
		t.Fatal(err)
	}

	sess := loginAndSession(t, ts.URL, "admin", "s3cret!")
	csrfTok := "testcsrf01234567890123456789012"
	chargeURL := ts.URL + "/api/admin/bookings/" + strconv.FormatInt(int64(b.ID), 10) + "/charge"

	req1 := mustNewRequest(t, http.MethodPost, chargeURL, strings.NewReader(`{"amount_cents":2000}`))
	addAdminCookies(req1, sess, csrfTok)
	req1.Header.Set("X-CSRF-Token", csrfTok)
	resp1 := doReq(t, ts.Client(), req1)
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp1.Body)
		t.Fatalf("first charge: status %d body %s", resp1.StatusCode, body)
	}
	if c := ms.chargeCount(); c != 1 {
		t.Fatalf("stripe charge calls after first request = %d, want 1", c)
	}

	req2 := mustNewRequest(t, http.MethodPost, chargeURL, strings.NewReader(`{"amount_cents":2000}`))
	addAdminCookies(req2, sess, csrfTok)
	req2.Header.Set("X-CSRF-Token", csrfTok)
	resp2 := doReq(t, ts.Client(), req2)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("second charge: status %d want 409 body %s", resp2.StatusCode, body)
	}
	if c := ms.chargeCount(); c != 1 {
		t.Fatalf("stripe charge calls after second request = %d, want 1 (no second Stripe call)", c)
	}
}

func TestAdminCSRF_mutatingWithoutHeader(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	ms := &mockStripe{}
	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	srv := New(testConfig(), q, ms, emailSvc, nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateAdminUser(ctx, "admin2", string(hash)); err != nil {
		t.Fatal(err)
	}
	sess := loginAndSession(t, ts.URL, "admin2", "s3cret!")
	csrfTok := "abccsrf012345678901234567890abc"

	u := ts.URL + "/api/admin/settings"
	req := mustNewRequest(t, http.MethodPut, u, strings.NewReader(`{"resource_name":"X"}`))
	addAdminCookies(req, sess, csrfTok)
	// Deliberately omit X-CSRF-Token
	resp := doReq(t, ts.Client(), req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d want 403 body %s", resp.StatusCode, body)
	}

	reqOK := mustNewRequest(t, http.MethodPut, u, strings.NewReader(`{"resource_name":"BookLab Test"}`))
	addAdminCookies(reqOK, sess, csrfTok)
	reqOK.Header.Set("X-CSRF-Token", csrfTok)
	respOK := doReq(t, ts.Client(), reqOK)
	defer respOK.Body.Close()
	if respOK.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respOK.Body)
		t.Fatalf("with CSRF: status %d want 200 body %s", respOK.StatusCode, body)
	}
}

func TestCORS_disallowedOriginNoACAO(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)

	ms := &mockStripe{}
	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	srv := New(testConfig(), q, ms, emailSvc, nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	req := mustNewRequest(t, http.MethodGet, ts.URL+"/api/settings/public", nil)
	req.Header.Set("Origin", "https://evil.example")
	resp := doReq(t, ts.Client(), req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	acao := resp.Header.Get("Access-Control-Allow-Origin")
	if acao == "https://evil.example" {
		t.Fatalf("disallowed origin was echoed in ACAO: %q", acao)
	}
}

func TestCORS_allowedOriginReflectsACAO(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)

	ms := &mockStripe{}
	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	srv := New(testConfig(), q, ms, emailSvc, nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	req := mustNewRequest(t, http.MethodGet, ts.URL+"/api/settings/public", nil)
	req.Header.Set("Origin", "https://trusted.example")
	resp := doReq(t, ts.Client(), req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if got, want := resp.Header.Get("Access-Control-Allow-Origin"), "https://trusted.example"; got != want {
		t.Fatalf("ACAO = %q, want %q", got, want)
	}
}

func TestAdminLogin_bruteForce429(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)

	ms := &mockStripe{}
	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	srv := New(testConfig(), q, ms, emailSvc, nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	client := ts.Client()

	for i := 0; i < 10; i++ {
		body := bytes.NewBufferString(`{"username":"nouser","password":"bad"}`)
		req := mustNewRequest(t, http.MethodPost, ts.URL+"/api/admin/login", body)
		req.RemoteAddr = "192.0.2.10:1234"
		resp := doReq(t, client, req)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d want 401", i+1, resp.StatusCode)
		}
	}

	body := bytes.NewBufferString(`{"username":"nouser","password":"bad"}`)
	req := mustNewRequest(t, http.MethodPost, ts.URL+"/api/admin/login", body)
	req.RemoteAddr = "192.0.2.10:1234"
	resp := doReq(t, client, req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("after threshold: status %d want 429 body %s", resp.StatusCode, b)
	}
}

func TestAdminLogout_revokesSession(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	ms := &mockStripe{}
	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	srv := New(testConfig(), q, ms, emailSvc, nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateAdminUser(ctx, "admin3", string(hash)); err != nil {
		t.Fatal(err)
	}
	sess := loginAndSession(t, ts.URL, "admin3", "s3cret!")
	csrfTok := "lgcsrf01234567890123456789012ab"

	getReq := mustNewRequest(t, http.MethodGet, ts.URL+"/api/admin/settings", nil)
	addAdminCookies(getReq, sess, csrfTok)
	if resp := doReq(t, ts.Client(), getReq); resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("before logout: %d", resp.StatusCode)
	} else {
		resp.Body.Close()
	}

	logoutReq := mustNewRequest(t, http.MethodPost, ts.URL+"/api/admin/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: "booklab_session", Value: sess})
	logoutResp := doReq(t, ts.Client(), logoutReq)
	logoutResp.Body.Close()
	if logoutResp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout status %d", logoutResp.StatusCode)
	}

	getAfter := mustNewRequest(t, http.MethodGet, ts.URL+"/api/admin/settings", nil)
	addAdminCookies(getAfter, sess, csrfTok)
	respAfter := doReq(t, ts.Client(), getAfter)
	defer respAfter.Body.Close()
	if respAfter.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(respAfter.Body)
		t.Fatalf("after logout want 401, got %d body %s", respAfter.StatusCode, b)
	}
}

func loginAndSession(t *testing.T, base, user, pass string) string {
	t.Helper()
	body := map[string]string{"username": user, "password": pass}
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, base+"/api/admin/login", bytes.NewReader(buf))
	resp := doReq(t, http.DefaultClient, req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("login: %d %s", resp.StatusCode, b)
	}
	for _, c := range resp.Cookies() {
		if c.Name == "booklab_session" && c.Value != "" {
			return c.Value
		}
	}
	t.Fatal("no booklab_session cookie from login")
	return "" // unreachable
}

func addAdminCookies(r *http.Request, session, csrf string) {
	r.AddCookie(&http.Cookie{Name: "booklab_session", Value: session})
	r.AddCookie(&http.Cookie{Name: "booklab_csrf", Value: csrf})
}

func mustNewRequest(t *testing.T, method, url string, body io.Reader) *http.Request {
	t.Helper()
	r, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	return r
}

func doReq(t *testing.T, client *http.Client, req *http.Request) *http.Response {
	t.Helper()
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
