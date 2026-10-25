package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	emailsvc "github.com/spencercornish/booklab/internal/email"
	"github.com/spencercornish/booklab/internal/db"
	"github.com/spencercornish/booklab/internal/stripe/stripetest"
)

// --- helpers ---------------------------------------------------------------

// setSecretLink flips the feature flags and (when token != "") issues the
// token. A nil token leaves any stored token untouched.
func setSecretLink(t *testing.T, ctx context.Context, q *db.Queries, enabled, anyHours bool, token *string) *db.Settings {
	t.Helper()
	if _, err := q.UpdateSettings(ctx, db.UpdateSettingsParams{
		SecretLinkEnabled:  &enabled,
		SecretLinkAnyHours: &anyHours,
	}); err != nil {
		t.Fatalf("update secret link settings: %v", err)
	}
	if token != nil {
		s, err := q.SetSecretLinkToken(ctx, *token)
		if err != nil {
			t.Fatalf("set secret link token: %v", err)
		}
		return s
	}
	s, err := q.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// disableSecretLink restores the default (off, no token) so a test cannot leak
// link state into the other suites sharing the settings row.
func disableSecretLink(t *testing.T, ctx context.Context, q *db.Queries) {
	t.Helper()
	off := false
	anyOff := false
	empty := ""
	if _, err := q.UpdateSettings(ctx, db.UpdateSettingsParams{
		SecretLinkEnabled:  &off,
		SecretLinkAnyHours: &anyOff,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SetSecretLinkToken(ctx, empty); err != nil {
		t.Fatal(err)
	}
}

func newSecretLinkServer(t *testing.T, q *db.Queries) *httptest.Server {
	t.Helper()
	srv := New(testConfig(), q, stripetest.New(), emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger()), nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func getAvailabilityWithToken(t *testing.T, baseURL, date, token string) availabilityResponse {
	t.Helper()
	u := baseURL + "/api/availability?date=" + date
	if token != "" {
		u += "&secret_token=" + token
	}
	req := mustNewRequest(t, http.MethodGet, u, nil)
	resp := doReq(t, http.DefaultClient, req)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("availability %s: status %d body %s", date, resp.StatusCode, body)
	}
	var got availabilityResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode availability: %v (%s)", err, body)
	}
	return got
}

func slotStartUTC(t *testing.T, raw string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("parse slot start %q: %v", raw, err)
	}
	return ts.UTC()
}

// postBookingWithToken posts a create-booking payload with an optional
// X-Secret-Token header and returns the status plus the decoded body.
func postBookingWithToken(t *testing.T, ts *httptest.Server, payload, token string) (int, map[string]any) {
	t.Helper()
	req := mustNewRequest(t, http.MethodPost, ts.URL+"/api/bookings", strings.NewReader(payload))
	if token != "" {
		req.Header.Set("X-Secret-Token", token)
	}
	resp := doReq(t, ts.Client(), req)
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

// secretLinkPayload builds a free booking create request for [start, end).
func secretLinkPayload(start, end time.Time) string {
	return fmt.Sprintf(`{"name":"Artist","email":"artist@example.com","start_time":%q,"end_time":%q}`,
		start.Format(time.RFC3339), end.Format(time.RFC3339))
}

const testLinkToken = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// --- verification & auth ----------------------------------------------------

func TestSecretLinkVerify_validTokenReturnsResource(t *testing.T) {
	ctx := context.Background()
	_, q := openTestPool(t)
	resetSettingsUTC(t, ctx, q)
	defer disableSecretLink(t, ctx, q)
	settings := setSecretLink(t, ctx, q, true, true, strptr(testLinkToken))
	ts := newSecretLinkServer(t, q)

	body := getJSONForTest(t, ts, "/api/secret-link/verify?token="+testLinkToken)
	if body["valid"] != true {
		t.Fatalf("valid = %v, want true", body["valid"])
	}
	if body["resource_name"] != settings.ResourceName {
		t.Fatalf("resource_name = %v, want %q", body["resource_name"], settings.ResourceName)
	}
	if body["any_hours"] != true {
		t.Fatalf("any_hours = %v, want true", body["any_hours"])
	}
}

func TestSecretLinkVerify_rejectsDisabledAndWrongToken(t *testing.T) {
	ctx := context.Background()
	_, q := openTestPool(t)
	resetSettingsUTC(t, ctx, q)
	defer disableSecretLink(t, ctx, q)
	setSecretLink(t, ctx, q, true, false, strptr(testLinkToken))
	ts := newSecretLinkServer(t, q)

	// Wrong token: uniform 200 with valid=false (no reason to 404 a probe).
	if body := getJSONForTest(t, ts, "/api/secret-link/verify?token=wrongwrongwrongwrongwrongwrongwrongwrong"); body["valid"] != false {
		t.Fatalf("wrong token: valid = %v, want false", body["valid"])
	}
	// Missing token.
	if body := getJSONForTest(t, ts, "/api/secret-link/verify"); body["valid"] != false {
		t.Fatalf("missing token: valid = %v, want false", body["valid"])
	}

	// Disabled but token still stored: invalid.
	setSecretLink(t, ctx, q, false, false, nil)
	if body := getJSONForTest(t, ts, "/api/secret-link/verify?token="+testLinkToken); body["valid"] != false {
		t.Fatalf("disabled: valid = %v, want false", body["valid"])
	}
}

func TestSecretLinkRotate_revokesOldTokenImmediately(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	defer disableSecretLink(t, ctx, q)
	setSecretLink(t, ctx, q, true, false, strptr(testLinkToken))
	ts := newSecretLinkServer(t, q)

	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateAdminUser(ctx, "admin", string(hash)); err != nil {
		t.Fatal(err)
	}
	sess := loginAndSession(t, ts.URL, "admin", "s3cret!")
	csrf := "csrfrotate0123456789012345678"

	req := mustNewRequest(t, http.MethodPost, ts.URL+"/api/admin/secret-link/rotate", nil)
	addAdminCookies(req, sess, csrf)
	req.Header.Set("X-CSRF-Token", csrf)
	resp := doReq(t, ts.Client(), req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("rotate: status %d body %s", resp.StatusCode, b)
	}
	var body struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Token) != 64 || body.URL == "" {
		t.Fatalf("rotate response = %+v, want 64-char token and url", body)
	}
	if !strings.HasSuffix(body.URL, "/book/"+body.Token) {
		t.Fatalf("url %q does not end with /book/<token>", body.URL)
	}

	// Old token is dead, the new one is live.
	if b := getJSONForTest(t, ts, "/api/secret-link/verify?token="+testLinkToken); b["valid"] != false {
		t.Fatalf("old token still valid after rotation")
	}
	if b := getJSONForTest(t, ts, "/api/secret-link/verify?token="+body.Token); b["valid"] != true {
		t.Fatalf("rotated token not valid: %v", b)
	}
}

// --- privacy pin -------------------------------------------------------------

// The link's existence is private: the public settings payload must never
// carry a secret_link key, even while the link is enabled and has a token.
func TestPublicSettings_omitSecretLink(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	defer disableSecretLink(t, ctx, q)
	setSecretLink(t, ctx, q, true, true, strptr(testLinkToken))
	ts := newSecretLinkServer(t, q)

	req := mustNewRequest(t, http.MethodGet, ts.URL+"/api/settings/public", nil)
	resp := doReq(t, ts.Client(), req)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret_link") {
		t.Fatalf("public settings leaked secret_link key: %s", raw)
	}
}

// --- admin settings round-trip ----------------------------------------------

func TestAdminSettings_secretLinkRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	defer disableSecretLink(t, ctx, q)
	ts := newSecretLinkServer(t, q)

	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateAdminUser(ctx, "admin", string(hash)); err != nil {
		t.Fatal(err)
	}
	sess := loginAndSession(t, ts.URL, "admin", "s3cret!")
	csrf := "csrfsettings01234567890123456"

	// Enabling bootstraps a token so the returned URL is immediately usable.
	req := mustNewRequest(t, http.MethodPut, ts.URL+"/api/admin/settings",
		strings.NewReader(`{"secret_link":{"enabled":true,"allow_outside_hours":true}}`))
	addAdminCookies(req, sess, csrf)
	req.Header.Set("X-CSRF-Token", csrf)
	resp := doReq(t, ts.Client(), req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("settings put: %d %s", resp.StatusCode, b)
	}
	var got struct {
		SecretLink struct {
			Enabled           bool    `json:"enabled"`
			AllowOutsideHours bool    `json:"allow_outside_hours"`
			Token             *string `json:"token"`
			URL               *string `json:"url"`
		} `json:"secret_link"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.SecretLink.Enabled || !got.SecretLink.AllowOutsideHours {
		t.Fatalf("flags not reflected: %+v", got.SecretLink)
	}
	if got.SecretLink.Token == nil || len(*got.SecretLink.Token) != 64 {
		t.Fatalf("enable did not bootstrap a 64-char token: %+v", got.SecretLink)
	}
	if got.SecretLink.URL == nil || !strings.HasSuffix(*got.SecretLink.URL, "/book/"+*got.SecretLink.Token) {
		t.Fatalf("url = %v", got.SecretLink.URL)
	}

	// Admin GET reflects the same state.
	reqG := mustNewRequest(t, http.MethodGet, ts.URL+"/api/admin/settings", nil)
	addAdminCookies(reqG, sess, csrf)
	respG := doReq(t, ts.Client(), reqG)
	defer respG.Body.Close()
	var adminGot struct {
		SecretLink struct {
			Enabled bool    `json:"enabled"`
			Token   *string `json:"token"`
		} `json:"secret_link"`
	}
	if err := json.NewDecoder(respG.Body).Decode(&adminGot); err != nil {
		t.Fatal(err)
	}
	if !adminGot.SecretLink.Enabled || adminGot.SecretLink.Token == nil || *adminGot.SecretLink.Token != *got.SecretLink.Token {
		t.Fatalf("admin GET secret_link = %+v, want enabled with same token", adminGot.SecretLink)
	}

	// The token survives a later unrelated settings update.
	req2 := mustNewRequest(t, http.MethodPut, ts.URL+"/api/admin/settings", strings.NewReader(`{"resource_name":"Renamed"}`))
	addAdminCookies(req2, sess, csrf)
	req2.Header.Set("X-CSRF-Token", csrf)
	resp2 := doReq(t, ts.Client(), req2)
	defer resp2.Body.Close()
	var got2 struct {
		SecretLink struct {
			Token *string `json:"token"`
		} `json:"secret_link"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&got2); err != nil {
		t.Fatal(err)
	}
	if got2.SecretLink.Token == nil || *got2.SecretLink.Token != *got.SecretLink.Token {
		t.Fatalf("unrelated settings update changed the token: %v vs %v", got2.SecretLink.Token, got.SecretLink.Token)
	}
}

// --- availability ------------------------------------------------------------

// With a valid token, slots inside the minimum-notice window are available;
// without it, the same slots are not. Link mode runs with any_hours so hourly
// slots exist at every wall-clock time the suite might run at: the with-token
// assertion alone proves the bypass (without it, earliest = now + lead would
// block those slots), and the without-token loop pins the standard rule when
// the lead window overlaps the bookable hours.
func TestSecretLinkAvailability_bypassesLeadTime(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	defer disableSecretLink(t, ctx, q)

	lead := int32(4)
	if _, err := q.UpdateSettings(ctx, db.UpdateSettingsParams{MinLeadHours: &lead}); err != nil {
		t.Fatal(err)
	}
	setSecretLink(t, ctx, q, true, true, strptr(testLinkToken))
	ts := newSecretLinkServer(t, q)

	now := time.Now().UTC()
	horizon := now.Add(time.Duration(lead) * time.Hour)
	today := now.Format("2006-01-02")
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")

	found := false
	for _, date := range []string{today, tomorrow} {
		without := getAvailabilityWithToken(t, ts.URL, date, "")
		with := getAvailabilityWithToken(t, ts.URL, date, testLinkToken)
		for _, s := range with.Slots {
			st := slotStartUTC(t, s.Start)
			if !(st.After(now) && !st.After(horizon)) {
				continue
			}
			found = true
			if !s.Available {
				t.Fatalf("slot %s inside lead window should be available with token", s.Start)
			}
		}
		for _, s := range without.Slots {
			st := slotStartUTC(t, s.Start)
			if st.After(now) && !st.After(horizon) && s.Available {
				t.Fatalf("slot %s inside lead window should be unavailable without token", s.Start)
			}
		}
	}
	if !found {
		t.Fatal("no slot fell inside the lead window; test window is broken")
	}
}

func TestSecretLinkAvailability_anyHours(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q) // bookable 10:00-16:00 UTC
	defer disableSecretLink(t, ctx, q)
	setSecretLink(t, ctx, q, true, false, strptr(testLinkToken))
	ts := newSecretLinkServer(t, q)

	day := "2030-06-15"
	withoutAny := getAvailabilityWithToken(t, ts.URL, day, testLinkToken)
	if len(withoutAny.Slots) != 6 {
		t.Fatalf("bookable-hours link slots = %d, want 6 (10:00-16:00)", len(withoutAny.Slots))
	}

	setSecretLink(t, ctx, q, true, true, nil)
	withAny := getAvailabilityWithToken(t, ts.URL, day, testLinkToken)
	if len(withAny.Slots) != 24 {
		t.Fatalf("any-hours link slots = %d, want 24", len(withAny.Slots))
	}
	for _, s := range withAny.Slots {
		hour := slotStartUTC(t, s.Start).Hour()
		if s.Available && (hour < 10 || hour >= 16) {
			// Off-hour slot: fine with any_hours.
			continue
		}
	}
	// Off-hour slots must exist and be available.
	for _, want := range []int{2, 22} {
		found := false
		for _, s := range withAny.Slots {
			if slotStartUTC(t, s.Start).Hour() == want {
				found = true
				if !s.Available {
					t.Fatalf("any-hours slot %02d:00 should be available", want)
				}
			}
		}
		if !found {
			t.Fatalf("any-hours slot %02d:00 missing", want)
		}
	}

	// Without a token the off-hour slots never appear (standard window).
	std := getAvailabilityWithToken(t, ts.URL, day, "")
	if len(std.Slots) != 6 {
		t.Fatalf("standard slots = %d, want 6", len(std.Slots))
	}
}

// DST pin: on the 2030 spring-forward date America/Denver has 23 real hours,
// so an any-hours link day yields 23 hourly slots, not 24.
func TestSecretLinkAvailability_springForwardDay(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	defer disableSecretLink(t, ctx, q)

	denver := "America/Denver"
	if _, err := q.UpdateSettings(ctx, db.UpdateSettingsParams{Timezone: &denver}); err != nil {
		t.Fatal(err)
	}
	setSecretLink(t, ctx, q, true, true, strptr(testLinkToken))
	ts := newSecretLinkServer(t, q)

	got := getAvailabilityWithToken(t, ts.URL, "2030-03-10", testLinkToken)
	if got.IsClosed {
		t.Fatal("spring-forward day must not be closed")
	}
	if len(got.Slots) != 23 {
		t.Fatalf("spring-forward any-hours slots = %d, want 23", len(got.Slots))
	}
	// Pinned in settings-local time: the nonexistent 02:00 MST must not appear;
	// 01:00 (MST, before the jump) and 03:00 (MDT, after it) both must.
	loc, err := time.LoadLocation("America/Denver")
	if err != nil {
		t.Fatal(err)
	}
	localHours := map[int]bool{}
	for _, s := range got.Slots {
		st, err := time.Parse(time.RFC3339, s.Start)
		if err != nil {
			t.Fatal(err)
		}
		localHours[st.In(loc).Hour()] = true
	}
	if localHours[2] {
		t.Fatalf("slot grid included the nonexistent 02:00 local slot: %v", localHours)
	}
	if !localHours[1] || !localHours[3] {
		t.Fatalf("01:00 and 03:00 local slots missing: %v", localHours)
	}
}

func TestSecretLinkAvailability_closuresAndConflictsStillApply(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	defer disableSecretLink(t, ctx, q)
	setSecretLink(t, ctx, q, true, true, strptr(testLinkToken))
	ts := newSecretLinkServer(t, q)

	day := "2030-06-20"
	// An existing booking blocks its slot even for link holders.
	conflictStart := time.Date(2030, 6, 20, 12, 0, 0, 0, time.UTC)
	if _, err := q.CreateBooking(ctx, db.CreateBookingParams{
		Name: "Existing", Email: "existing@example.com",
		Start:           conflictStart,
		End:             conflictStart.Add(2 * time.Hour),
		SetupIntentID:   strptr("si_conflict"),
		PaymentMethodID: strptr("pm_conflict"),
	}); err != nil {
		t.Fatal(err)
	}
	// A partial closure blocks 15:00-16:00.
	cStart := time.Date(0, 1, 1, 15, 0, 0, 0, time.UTC)
	cEnd := time.Date(0, 1, 1, 16, 0, 0, 0, time.UTC)
	if _, err := q.CreateClosure(ctx, time.Date(2030, 6, 20, 0, 0, 0, 0, time.UTC), time.Date(2030, 6, 20, 0, 0, 0, 0, time.UTC), false, &cStart, &cEnd, nil); err != nil {
		t.Fatal(err)
	}

	got := getAvailabilityWithToken(t, ts.URL, day, testLinkToken)
	for _, s := range got.Slots {
		st := slotStartUTC(t, s.Start)
		hour := st.Hour()
		switch hour {
		case 12, 13:
			if s.Available {
				t.Fatalf("slot %s overlaps an existing booking and must be unavailable", s.Start)
			}
		case 15:
			if s.Available {
				t.Fatalf("slot %s overlaps a partial closure and must be unavailable", s.Start)
			}
		}
	}
}

// --- creation ----------------------------------------------------------------

// Free bookings skip the minimum-notice rule: a token-holder can book right up
// to the (future) start time, while the same request without a token gets the
// standard lead-time rejection.
func TestSecretLinkCreate_acceptsInsideLeadWindow(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	defer disableSecretLink(t, ctx, q)

	lead := int32(24)
	if _, err := q.UpdateSettings(ctx, db.UpdateSettingsParams{MinLeadHours: &lead}); err != nil {
		t.Fatal(err)
	}
	setSecretLink(t, ctx, q, true, true, strptr(testLinkToken)) // any_hours keeps the slot in-bounds at any wall clock
	ts := newSecretLinkServer(t, q)

	start := time.Now().Add(2 * time.Hour).Truncate(time.Hour)
	payload := secretLinkPayload(start, start.Add(2*time.Hour))

	if status, body := postBookingWithToken(t, ts, payload, testLinkToken); status != http.StatusCreated {
		t.Fatalf("with token: status %d body %v, want 201", status, body)
	}

	// Same slot, no token: the lead-time rule bites again.
	start2 := time.Now().Add(3 * time.Hour).Truncate(time.Hour)
	payload2 := fmt.Sprintf(`{"setup_intent_id":%q,"name":"Pat","email":"pat@example.com","start_time":%q,"end_time":%q}`,
		mintSetupIntentID(t), start2.Format(time.RFC3339), start2.Add(2*time.Hour).Format(time.RFC3339))
	if status, body := postBookingWithToken(t, ts, payload2, ""); status != http.StatusBadRequest {
		t.Fatalf("without token: status %d body %v, want 400 (lead time)", status, body)
	}

	// A bogus token must not unlock the free path (and no card means 400).
	if status, body := postBookingWithToken(t, ts, secretLinkPayload(start2, start2.Add(2*time.Hour)), "bogus"); status != http.StatusBadRequest {
		t.Fatalf("bogus token: status %d body %v, want 400", status, body)
	}
}

func TestSecretLinkCreate_persistsFreeBooking(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	defer disableSecretLink(t, ctx, q)
	setSecretLink(t, ctx, q, true, true, strptr(testLinkToken))
	ts := newSecretLinkServer(t, q)

	start := time.Date(2035, 6, 10, 12, 0, 0, 0, time.UTC)
	status, body := postBookingWithToken(t, ts, secretLinkPayload(start, start.Add(2*time.Hour)), testLinkToken)
	if status != http.StatusCreated {
		t.Fatalf("status %d body %v, want 201", status, body)
	}
	if body["booking"].(map[string]any)["status"] != "confirmed" {
		t.Fatalf("booking status = %v, want confirmed", body)
	}

	bookings, err := q.ListBookings(ctx, db.ListBookingsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bookings) != 1 {
		t.Fatalf("bookings = %d, want 1", len(bookings))
	}
	b := bookings[0]
	if !b.ViaSecretLink {
		t.Fatal("via_secret_link = false, want true")
	}
	if b.StripeSetupIntentID != nil || b.StripePaymentMethodID != nil {
		t.Fatalf("free booking stored stripe fields: si=%v pm=%v", b.StripeSetupIntentID, b.StripePaymentMethodID)
	}
	if b.IsNewUser != nil {
		t.Fatal("free booking should not store a new-user answer")
	}
}

func TestSecretLinkCreate_stillEnforcesDurationsAndClosures(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q) // min 2h, max 8h
	defer disableSecretLink(t, ctx, q)
	setSecretLink(t, ctx, q, true, true, strptr(testLinkToken))
	ts := newSecretLinkServer(t, q)

	day := time.Date(2035, 7, 1, 0, 0, 0, 0, time.UTC)

	// Under min duration.
	start := day.Add(12 * time.Hour)
	if status, _ := postBookingWithToken(t, ts, secretLinkPayload(start, start.Add(time.Hour)), testLinkToken); status != http.StatusBadRequest {
		t.Fatalf("1h booking: want 400")
	}
	// Over max duration.
	if status, _ := postBookingWithToken(t, ts, secretLinkPayload(start, start.Add(9*time.Hour)), testLinkToken); status != http.StatusBadRequest {
		t.Fatalf("9h booking: want 400")
	}
	// Past start.
	past := time.Now().Add(-2 * time.Hour)
	if status, _ := postBookingWithToken(t, ts, secretLinkPayload(past, past.Add(2*time.Hour)), testLinkToken); status != http.StatusBadRequest {
		t.Fatalf("past booking: want 400")
	}
	// Closure overlap.
	if _, err := q.CreateClosure(ctx, day, day, true, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if status, _ := postBookingWithToken(t, ts, secretLinkPayload(start, start.Add(2*time.Hour)), testLinkToken); status != http.StatusBadRequest {
		t.Fatalf("closure overlap: want 400")
	}
}

// The free flow has no idempotency key: a retry after a succeeded create hits
// the overlap constraint and 409s instead of returning the booking.
func TestSecretLinkCreate_retryAfterSuccessConflicts(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	defer disableSecretLink(t, ctx, q)
	setSecretLink(t, ctx, q, true, true, strptr(testLinkToken))
	ts := newSecretLinkServer(t, q)

	start := time.Date(2035, 6, 11, 12, 0, 0, 0, time.UTC)
	payload := secretLinkPayload(start, start.Add(2*time.Hour))

	if status, _ := postBookingWithToken(t, ts, payload, testLinkToken); status != http.StatusCreated {
		t.Fatalf("first create: want 201")
	}
	status, body := postBookingWithToken(t, ts, payload, testLinkToken)
	if status != http.StatusConflict {
		t.Fatalf("retry: status %d body %v, want 409", status, body)
	}
	bookings, err := q.ListBookings(ctx, db.ListBookingsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bookings) != 1 {
		t.Fatalf("bookings = %d, want 1 (no duplicate row)", len(bookings))
	}
}

// --- lifecycle & charge safety ----------------------------------------------

func TestCompleteExpiredBookings_includesFreeBookings(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	// A past free booking (no payment method) must be auto-completed. The
	// three bookings get disjoint intervals (the overlap constraint is global).
	past := time.Now().Add(-48 * time.Hour)
	free, err := q.CreateBooking(ctx, db.CreateBookingParams{
		Name: "Artist", Email: "artist@example.com",
		Start: past, End: past.Add(2 * time.Hour),
		ViaSecretLink: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// A past paid booking still completes as before.
	paid, err := q.CreateBooking(ctx, db.CreateBookingParams{
		Name: "Paid", Email: "paid@example.com",
		Start: past.Add(3 * time.Hour), End: past.Add(5 * time.Hour),
		SetupIntentID:   strptr("si_done"),
		PaymentMethodID: strptr("pm_done"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// A past booking with neither marker stays untouched (data-integrity guard).
	orphan, err := q.CreateBooking(ctx, db.CreateBookingParams{
		Name: "Orphan", Email: "orphan@example.com",
		Start: past.Add(6 * time.Hour), End: past.Add(8 * time.Hour),
		SetupIntentID: strptr("si_orphan2"),
	})
	if err != nil {
		t.Fatal(err)
	}

	completed, err := q.CompleteExpiredBookings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int32]bool{}
	for _, b := range completed {
		got[b.ID] = true
	}
	if !got[free.ID] || !got[paid.ID] {
		t.Fatalf("expected free %d and paid %d to complete, got %v", free.ID, paid.ID, got)
	}
	if got[orphan.ID] {
		t.Fatalf("booking %d has no payment method and no link marker; it must not complete", orphan.ID)
	}
}

func TestSecretLinkCreate_freeBookingNeverAutoCharges(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	past := time.Now().Add(-48 * time.Hour)
	b, err := q.CreateBooking(ctx, db.CreateBookingParams{
		Name: "Artist", Email: "artist@example.com",
		Start: past, End: past.Add(2 * time.Hour),
		ViaSecretLink: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CompleteExpiredBookings(ctx); err != nil {
		t.Fatal(err)
	}

	// The auto-charge selectors require a payment method on file, so the free
	// booking is never claimed.
	due, err := q.ListBookingsDueAutoCharge(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range due {
		if d.ID == b.ID {
			t.Fatalf("free booking %d appeared in auto-charge list", b.ID)
		}
	}
	if _, err := q.ClaimBookingForAutoCharge(ctx, 0); err != pgx.ErrNoRows {
		t.Fatalf("ClaimBookingForAutoCharge = %v, want ErrNoRows", err)
	}
}

func TestAdminUpdateBooking_freeBookingCanComplete(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	ts := newSecretLinkServer(t, q)

	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateAdminUser(ctx, "admin", string(hash)); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2031, 5, 1, 12, 0, 0, 0, time.UTC)
	free, err := q.CreateBooking(ctx, db.CreateBookingParams{
		Name: "Artist", Email: "artist@example.com",
		Start: start, End: start.Add(2 * time.Hour),
		ViaSecretLink: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	sess := loginAndSession(t, ts.URL, "admin", "s3cret!")
	csrf := "csrfcomplete01234567890123456"
	req := mustNewRequest(t, http.MethodPatch, ts.URL+"/api/admin/bookings/"+strconv.FormatInt(int64(free.ID), 10),
		strings.NewReader(`{"status":"completed"}`))
	addAdminCookies(req, sess, csrf)
	req.Header.Set("X-CSRF-Token", csrf)
	resp := doReq(t, ts.Client(), req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("complete free booking: status %d body %s", resp.StatusCode, b)
	}
}

func TestAdminChargeBooking_freeBookingIsBadRequest(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	ts := newSecretLinkServer(t, q)

	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateAdminUser(ctx, "admin", string(hash)); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2031, 5, 2, 12, 0, 0, 0, time.UTC)
	free, err := q.CreateBooking(ctx, db.CreateBookingParams{
		Name: "Artist", Email: "artist@example.com",
		Start: start, End: start.Add(2 * time.Hour),
		ViaSecretLink: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateBookingStatus(ctx, free.ID, db.BookingStatusCompleted); err != nil {
		t.Fatal(err)
	}

	sess := loginAndSession(t, ts.URL, "admin", "s3cret!")
	csrf := "csrfcharge0123456789012345678"
	req := mustNewRequest(t, http.MethodPost, ts.URL+"/api/admin/bookings/"+strconv.FormatInt(int64(free.ID), 10)+"/charge",
		strings.NewReader(`{"amount_cents":2000}`))
	addAdminCookies(req, sess, csrf)
	req.Header.Set("X-CSRF-Token", csrf)
	resp := doReq(t, ts.Client(), req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("charge free booking: status %d body %s, want 400 (never 500/panic)", resp.StatusCode, b)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "free studio booking cannot be charged" {
		t.Fatalf("error = %q", body.Error)
	}

	// The row must not be left in the charging state.
	after, err := q.GetBookingByID(ctx, free.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != db.BookingStatusCompleted {
		t.Fatalf("status after rejected charge = %s, want completed", after.Status)
	}
}
