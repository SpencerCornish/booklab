package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	emailsvc "github.com/spencercornish/booklab/internal/email"

	"github.com/spencercornish/booklab/internal/db"
)

func boolPtr(b bool) *bool { return &b }

// gateTestDate is a fixed future date; tests derive its weekday instead of
// hardcoding it.
var gateTestDate = time.Date(2035, 7, 10, 0, 0, 0, 0, time.UTC)

func TestNewUserWindowsForDate_usesWeekdaySchedule(t *testing.T) {
	settings := &db.Settings{
		Timezone: "UTC",
		NewUserGateSchedule: []db.NewUserWindow{
			{Weekday: int(gateTestDate.Weekday()), Start: "18:00", End: "21:00"},
			{Weekday: (int(gateTestDate.Weekday()) + 1) % 7, Start: "10:00", End: "12:00"},
		},
	}
	windows := newUserWindowsForDate(settings, gateTestDate, time.UTC)
	if len(windows) != 1 {
		t.Fatalf("windows = %v, want 1", windows)
	}
	if windows[0].Start.Hour() != 18 || windows[0].End.Hour() != 21 {
		t.Fatalf("window = %v–%v, want 18:00–21:00", windows[0].Start, windows[0].End)
	}

	// A different weekday has no windows configured.
	other := gateTestDate.AddDate(0, 0, 2)
	if got := newUserWindowsForDate(settings, other, time.UTC); len(got) != 0 {
		t.Fatalf("windows = %v, want none", got)
	}
}

func TestNewUserWindowsForDate_skipsInvalidAndMerges(t *testing.T) {
	wd := int(gateTestDate.Weekday())
	settings := &db.Settings{
		Timezone: "UTC",
		NewUserGateSchedule: []db.NewUserWindow{
			{Weekday: wd, Start: "13:00", End: "15:00"},
			{Weekday: wd, Start: "10:00", End: "12:00"},
			{Weekday: wd, Start: "12:00", End: "12:30"}, // adjacent, merges into the second window
			{Weekday: wd, Start: "25:00", End: "26:00"}, // invalid, skipped
			{Weekday: wd, Start: "18:00", End: "17:00"}, // inverted, skipped
		},
	}
	windows := newUserWindowsForDate(settings, gateTestDate, time.UTC)
	if len(windows) != 2 {
		t.Fatalf("windows = %v, want 2", windows)
	}
	if windows[0].Start.Hour() != 10 || windows[0].End.Hour() != 12 || windows[0].End.Minute() != 30 {
		t.Fatalf("first window = %v–%v, want 10:00–12:30", windows[0].Start, windows[0].End)
	}
	if windows[1].Start.Hour() != 13 || windows[1].End.Hour() != 15 {
		t.Fatalf("second window = %v–%v, want 13:00–15:00", windows[1].Start, windows[1].End)
	}
}

func TestValidateBookingCreate_newUserRejectedOutsideTrainerHours(t *testing.T) {
	s := &Server{cfg: testConfig(), logger: testLogger()}
	// Configure trainer hours on a weekday other than the booking date's.
	settings := &db.Settings{
		Timezone:           "UTC",
		MinHours:           1,
		MaxHours:           8,
		NewUserGateEnabled: true,
		NewUserGateSchedule: []db.NewUserWindow{
			{Weekday: (int(gateTestDate.Weekday()) + 1) % 7, Start: "10:00", End: "21:00"},
		},
	}
	start := gateTestDate.Add(10 * time.Hour)
	bad, err := s.validateBookingCreate(context.Background(), start, start.Add(2*time.Hour), settings, true)
	if err != nil {
		t.Fatal(err)
	}
	if bad != "new customers can only book during trainer hours" {
		t.Fatalf("got %q", bad)
	}
}

func TestParseNewUserGateSchedule_validatesAndNormalises(t *testing.T) {
	out, bad := parseNewUserGateSchedule([]db.NewUserWindow{
		{Weekday: 3, Start: "15:30", End: "20:00"},
		{Weekday: 1, Start: "09:00", End: "17:00"},
		{Weekday: 3, Start: "14:00", End: "16:00"}, // overlaps the Wednesday window
	})
	if bad == "" {
		t.Fatal("expected overlap rejection")
	}

	out, bad = parseNewUserGateSchedule([]db.NewUserWindow{
		{Weekday: 3, Start: "15:30", End: "20:00"},
		{Weekday: 1, Start: "09:00", End: "17:00"},
	})
	if bad != "" {
		t.Fatalf("unexpected rejection: %s", bad)
	}
	if len(out) != 2 {
		t.Fatalf("windows = %v, want 2", out)
	}
	// Sorted by weekday, times normalised to HH:MM.
	if out[0].Weekday != 1 || out[0].Start != "09:00" || out[0].End != "17:00" {
		t.Fatalf("first window = %+v", out[0])
	}
	if out[1].Weekday != 3 || out[1].Start != "15:30" || out[1].End != "20:00" {
		t.Fatalf("second window = %+v", out[1])
	}

	if _, bad := parseNewUserGateSchedule([]db.NewUserWindow{{Weekday: 7, Start: "09:00", End: "17:00"}}); bad == "" {
		t.Fatal("expected weekday rejection")
	}
	if _, bad := parseNewUserGateSchedule([]db.NewUserWindow{{Weekday: 1, Start: "9am", End: "17:00"}}); bad == "" {
		t.Fatal("expected time format rejection")
	}
	if _, bad := parseNewUserGateSchedule([]db.NewUserWindow{{Weekday: 1, Start: "17:00", End: "09:00"}}); bad == "" {
		t.Fatal("expected inverted window rejection")
	}
}

func enableNewUserGate(t *testing.T, ctx context.Context, q *db.Queries, question string, schedule []db.NewUserWindow) {
	t.Helper()
	enabled := true
	params := db.UpdateSettingsParams{
		NewUserGateEnabled:  &enabled,
		NewUserGateSchedule: &schedule,
	}
	if question != "" {
		params.NewUserGateQuestion = &question
	}
	if _, err := q.UpdateSettings(ctx, params); err != nil {
		t.Fatalf("enable gate: %v", err)
	}
}

func TestAvailability_newUserSeesTrainerSlots(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	enableNewUserGate(t, ctx, q, "", []db.NewUserWindow{{Weekday: int(gateTestDate.Weekday()), Start: "18:00", End: "21:00"}})

	// An existing (experienced) booking blocks the trainer slot for everyone.
	if _, err := q.CreateBooking(ctx, "Pat", "pat@example.com", nil,
		gateTestDate.Add(19*time.Hour), gateTestDate.Add(20*time.Hour), "si_gate_block", "pm_test_123", boolPtr(false)); err != nil {
		t.Fatal(err)
	}

	srv := New(testConfig(), q, &mockStripe{}, emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger()), nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	fetch := func(query string) (isClosed bool, slots []dbAvailabilitySlot) {
		t.Helper()
		req := mustNewRequest(t, http.MethodGet, ts.URL+"/api/availability?date="+gateTestDate.Format("2006-01-02")+query, nil)
		resp := doReq(t, ts.Client(), req)
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d body %s", resp.StatusCode, b)
		}
		var body struct {
			IsClosed bool                 `json:"is_closed"`
			Slots    []dbAvailabilitySlot `json:"slots"`
		}
		if err := json.Unmarshal(b, &body); err != nil {
			t.Fatalf("decode availability: %v (%s)", err, b)
		}
		return body.IsClosed, body.Slots
	}

	// Experienced customers see the normal window 10:00–16:00.
	closed, slots := fetch("")
	if closed {
		t.Fatal("date should not be closed for experienced customers")
	}
	if len(slots) != 6 {
		t.Fatalf("normal slots = %d, want 6", len(slots))
	}
	if !slots[0].Start.Equal(gateTestDate.Add(10 * time.Hour)) {
		t.Fatalf("first normal slot = %v, want 10:00", slots[0].Start)
	}

	// New customers see trainer hours 18:00–21:00, with the booked hour blocked.
	closed, slots = fetch("&new_user=true")
	if closed {
		t.Fatal("date should not be closed for new customers")
	}
	if len(slots) != 3 {
		t.Fatalf("trainer slots = %d, want 3", len(slots))
	}
	if !slots[0].Start.Equal(gateTestDate.Add(18 * time.Hour)) {
		t.Fatalf("first trainer slot = %v, want 18:00", slots[0].Start)
	}
	if slots[1].Available {
		t.Fatalf("19:00 trainer slot should be blocked by an existing booking")
	}
	if !slots[0].Available || !slots[2].Available {
		t.Fatalf("18:00 and 20:00 trainer slots should be available")
	}
}

type dbAvailabilitySlot struct {
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Available bool      `json:"available"`
}

func TestAvailability_newUserNoTrainerWindowReturnsNoSlots(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	enableNewUserGate(t, ctx, q, "", []db.NewUserWindow{{Weekday: (int(gateTestDate.Weekday()) + 1) % 7, Start: "10:00", End: "12:00"}})

	srv := New(testConfig(), q, &mockStripe{}, emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger()), nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	req := mustNewRequest(t, http.MethodGet, ts.URL+"/api/availability?date="+gateTestDate.Format("2006-01-02")+"&new_user=true", nil)
	resp := doReq(t, ts.Client(), req)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
	var body struct {
		IsClosed bool                 `json:"is_closed"`
		Slots    []dbAvailabilitySlot `json:"slots"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		t.Fatalf("decode availability: %v (%s)", err, b)
	}
	if body.IsClosed {
		t.Fatal("a day without trainer hours is not a closure")
	}
	if len(body.Slots) != 0 {
		t.Fatalf("slots = %v, want none", body.Slots)
	}
}

func TestCreateBooking_newUserValidationAndPersistence(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	// Trainer hours 18:00–21:00, outside the normal 10:00–16:00 window.
	enableNewUserGate(t, ctx, q, "Have you trained here before?", []db.NewUserWindow{{Weekday: int(gateTestDate.Weekday()), Start: "18:00", End: "21:00"}})

	srv := New(testConfig(), q, &mockStripe{}, emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger()), nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	create := func(payload string) (int, map[string]any) {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, ts.URL+"/api/bookings", strings.NewReader(payload))
		resp := doReq(t, ts.Client(), req)
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		return resp.StatusCode, body
	}

	insideStart := gateTestDate.Add(19 * time.Hour)
	insideEnd := insideStart.Add(2 * time.Hour)
	payload := func(si string, start, end time.Time, extra string) string {
		return fmt.Sprintf(`{"setup_intent_id":"%s","name":"Pat","email":"pat@example.com","start_time":%q,"end_time":%q%s}`,
			si, start.Format(time.RFC3339), end.Format(time.RFC3339), extra)
	}

	// Missing answer while the gate is enabled.
	code, body := create(payload("si_gate_missing", insideStart, insideEnd, ""))
	if code != http.StatusBadRequest || body["error"] != "please answer the pre-booking question" {
		t.Fatalf("missing answer: status %d body %v", code, body)
	}

	// Answering "new" but booking outside trainer hours (normal hours are
	// irrelevant for new users, so 13:00–15:00 must be rejected too).
	code, body = create(payload("si_gate_outside", gateTestDate.Add(13*time.Hour), gateTestDate.Add(15*time.Hour), `,"new_user":true`))
	if code != http.StatusBadRequest {
		t.Fatalf("outside trainer hours: status %d body %v", code, body)
	}
	if msg, _ := body["error"].(string); msg != "new customers can only book during trainer hours" {
		t.Fatalf("outside trainer hours message: %v", body["error"])
	}

	// Answering "new" inside trainer hours stores the answer.
	code, body = create(payload("si_gate_inside", insideStart, insideEnd, `,"new_user":true`))
	if code != http.StatusCreated {
		t.Fatalf("inside trainer hours: status %d body %v", code, body)
	}
	created, err := q.GetBookingBySetupIntentID(ctx, "si_gate_inside")
	if err != nil {
		t.Fatal(err)
	}
	if created.IsNewUser == nil || !*created.IsNewUser {
		t.Fatalf("is_new_user = %v, want true", created.IsNewUser)
	}

	// Experienced answer inside normal hours stores false.
	expStart := gateTestDate.Add(12 * time.Hour)
	expEnd := expStart.Add(2 * time.Hour)
	code, _ = create(payload("si_gate_exp", expStart, expEnd, `,"new_user":false`))
	if code != http.StatusCreated {
		t.Fatalf("experienced booking: status %d", code)
	}
	experienced, err := q.GetBookingBySetupIntentID(ctx, "si_gate_exp")
	if err != nil {
		t.Fatal(err)
	}
	if experienced.IsNewUser == nil || *experienced.IsNewUser {
		t.Fatalf("is_new_user = %v, want false", experienced.IsNewUser)
	}
}

func TestPublicSettings_newUserGate(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	srv := New(testConfig(), q, &mockStripe{}, emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger()), nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	getGate := func(t *testing.T) *struct {
		Enabled  bool   `json:"enabled"`
		Question string `json:"question"`
	} {
		t.Helper()
		req := mustNewRequest(t, http.MethodGet, ts.URL+"/api/settings/public", nil)
		resp := doReq(t, ts.Client(), req)
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d body %s", resp.StatusCode, b)
		}
		var body struct {
			NewUserGate *struct {
				Enabled  bool   `json:"enabled"`
				Question string `json:"question"`
			} `json:"new_user_gate"`
		}
		if err := json.Unmarshal(b, &body); err != nil {
			t.Fatalf("decode settings: %v (%s)", err, b)
		}
		return body.NewUserGate
	}

	if gate := getGate(t); gate != nil {
		t.Fatalf("gate should be hidden while disabled, got %+v", gate)
	}

	enableNewUserGate(t, ctx, q, "", []db.NewUserWindow{{Weekday: 2, Start: "15:00", End: "20:00"}})
	gate := getGate(t)
	if gate == nil || !gate.Enabled {
		t.Fatalf("gate should be advertised while enabled")
	}
	if gate.Question != defaultNewUserGateQuestion {
		t.Fatalf("question = %q, want default", gate.Question)
	}

	enableNewUserGate(t, ctx, q, "Have you trained here before?", nil)
	gate = getGate(t)
	if gate == nil || gate.Question != "Have you trained here before?" {
		t.Fatalf("question = %+v, want custom text", gate)
	}
}

func TestAdminSettings_newUserGate(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateAdminUser(ctx, "admin", string(hash)); err != nil {
		t.Fatal(err)
	}
	ts := newCustomerTestServer(t, q)
	sess := loginAndSession(t, ts.URL, "admin", "s3cret!")
	csrfTok := "testcsrf01234567890123456789012"

	put := func(payload string) (int, map[string]any) {
		req := mustNewRequest(t, http.MethodPut, ts.URL+"/api/admin/settings", strings.NewReader(payload))
		addAdminCookies(req, sess, csrfTok)
		req.Header.Set("X-CSRF-Token", csrfTok)
		resp := doReq(t, ts.Client(), req)
		defer resp.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body
	}

	status, body := put(`{"new_user_gate":{"enabled":true,"question":"Have you trained here before?","schedule":[{"weekday":2,"start":"15:00","end":"20:00"}]}}`)
	if status != http.StatusOK {
		t.Fatalf("set gate: status %d body %v", status, body)
	}
	got, _ := body["new_user_gate"].(map[string]any)
	if got == nil || got["enabled"] != true || got["question"] != "Have you trained here before?" {
		t.Fatalf("set gate: response gate = %v", got)
	}
	schedule, _ := got["schedule"].([]any)
	if len(schedule) != 1 {
		t.Fatalf("set gate: schedule = %v, want 1 window", schedule)
	}

	// An absent field leaves the gate unchanged.
	status, body = put(`{"min_hours":2}`)
	if status != http.StatusOK || body["new_user_gate"] == nil {
		t.Fatalf("absent field: status %d gate %v, want unchanged", status, body["new_user_gate"])
	}

	// A blank question falls back to the default, and an empty schedule clears it.
	status, body = put(`{"new_user_gate":{"enabled":true,"question":"","schedule":[]}}`)
	if status != http.StatusOK {
		t.Fatalf("blank question: status %d body %v", status, body)
	}
	got, _ = body["new_user_gate"].(map[string]any)
	if got["question"] != defaultNewUserGateQuestion {
		t.Fatalf("blank question: response question = %v, want default", got["question"])
	}

	// Overlapping windows on the same weekday are rejected.
	if status, _ := put(`{"new_user_gate":{"enabled":true,"question":"q","schedule":[{"weekday":2,"start":"15:00","end":"20:00"},{"weekday":2,"start":"19:00","end":"21:00"}]}}`); status != http.StatusBadRequest {
		t.Fatalf("overlapping windows: status %d, want 400", status)
	}

	stored, err := q.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.NewUserGateEnabled || stored.NewUserGateQuestion != defaultNewUserGateQuestion {
		t.Fatalf("stored gate = enabled %v question %q", stored.NewUserGateEnabled, stored.NewUserGateQuestion)
	}
	if len(stored.NewUserGateSchedule) != 0 {
		t.Fatalf("stored schedule = %v, want empty", stored.NewUserGateSchedule)
	}
}

func TestAvailableDates_trainerWindowsClosuresAndBookings(t *testing.T) {
	bs := time.Date(0, 1, 1, 10, 0, 0, 0, time.UTC)
	be := time.Date(0, 1, 1, 16, 0, 0, 0, time.UTC)
	settings := &db.Settings{
		Timezone:            "UTC",
		BookableStart:       bs,
		BookableEnd:         be,
		MinHours:            2,
		MaxHours:            8,
		NewUserGateSchedule: []db.NewUserWindow{{Weekday: 2, Start: "15:00", End: "20:00"}},
	}
	// The one Tuesday inside the 7-day window starting at gateTestDate.
	tue := gateTestDate.AddDate(0, 0, (int(time.Tuesday)-int(gateTestDate.Weekday())+7)%7)
	tueStr := tue.Format("2006-01-02")

	// Only the Tuesday has trainer hours.
	dates := availableDates(settings, nil, nil, gateTestDate, 7, time.UTC, true)
	if len(dates) != 1 || dates[0] != tueStr {
		t.Fatalf("dates = %v, want [%s]", dates, tueStr)
	}

	// An all-day closure removes it.
	closures := []*db.Closure{{AllDay: true, StartDate: tue, EndDate: tue.AddDate(0, 0, 1)}}
	if dates := availableDates(settings, closures, nil, gateTestDate, 7, time.UTC, true); len(dates) != 0 {
		t.Fatalf("dates with closure = %v, want none", dates)
	}

	// A 2-hour booking still leaves 3 contiguous hours, above the 2-hour minimum.
	bookings := []*db.Booking{{StartTime: tue.Add(15 * time.Hour), EndTime: tue.Add(17 * time.Hour)}}
	if dates := availableDates(settings, nil, bookings, gateTestDate, 7, time.UTC, true); len(dates) != 1 || dates[0] != tueStr {
		t.Fatalf("dates with partial booking = %v, want [%s]", dates, tueStr)
	}

	// Booking all but one hour leaves no 2-hour run, so the day is dropped.
	bookings = []*db.Booking{{StartTime: tue.Add(15 * time.Hour), EndTime: tue.Add(19 * time.Hour)}}
	if dates := availableDates(settings, nil, bookings, gateTestDate, 7, time.UTC, true); len(dates) != 0 {
		t.Fatalf("dates with near-full booking = %v, want none", dates)
	}

	// Without the gate routing, normal hours apply to every day.
	if dates := availableDates(settings, nil, nil, gateTestDate, 7, time.UTC, false); len(dates) != 7 {
		t.Fatalf("normal dates = %v, want all 7", dates)
	}
}

func TestAvailableDaysEndpoint_newUserOnlyTrainerDays(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	enableNewUserGate(t, ctx, q, "", []db.NewUserWindow{
		{Weekday: 2, Start: "15:00", End: "20:00"},
		{Weekday: 4, Start: "15:00", End: "20:00"},
	})

	// Book out every trainer hour on a Thursday far enough out that the result
	// cannot depend on what time of day the test runs.
	today := time.Now().UTC()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	offset := func(weekday time.Weekday, minDays int) time.Time {
		off := (int(weekday) - int(today.Weekday()) + 7) % 7
		if off < minDays {
			off += 7
		}
		return today.AddDate(0, 0, off)
	}
	thu := offset(time.Thursday, 7)
	if _, err := q.CreateBooking(ctx, "Pat", "pat@example.com", nil,
		thu.Add(15*time.Hour), thu.Add(20*time.Hour), "si_days_full", "pm_test_123", boolPtr(true)); err != nil {
		t.Fatal(err)
	}

	srv := New(testConfig(), q, &mockStripe{}, emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger()), nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	fetch := func(query string) []string {
		t.Helper()
		req := mustNewRequest(t, http.MethodGet, ts.URL+"/api/availability/days?days=21"+query, nil)
		resp := doReq(t, ts.Client(), req)
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d body %s", resp.StatusCode, b)
		}
		var body struct {
			AvailableDates []string `json:"available_dates"`
		}
		if err := json.Unmarshal(b, &body); err != nil {
			t.Fatalf("decode: %v (%s)", err, b)
		}
		return body.AvailableDates
	}

	// New users: exactly the Tuesdays and Thursdays, minus the booked-out one.
	dates := fetch("&new_user=true")
	tueWant := offset(time.Tuesday, 7).Format("2006-01-02")
	thuWant := thu.Format("2006-01-02")
	seen := map[string]bool{}
	for _, d := range dates {
		parsed, err := time.Parse("2006-01-02", d)
		if err != nil {
			t.Fatalf("bad date %q: %v", d, err)
		}
		if wd := parsed.Weekday(); wd != time.Tuesday && wd != time.Thursday {
			t.Fatalf("date %s is a %s, want Tue/Thu only", d, wd)
		}
		seen[d] = true
	}
	if !seen[tueWant] {
		t.Fatalf("expected trainer Tuesday %s in %v", tueWant, dates)
	}
	if seen[thuWant] {
		t.Fatalf("booked-out Thursday %s should be absent from %v", thuWant, dates)
	}

	// Experienced users still see the normal bookable days.
	normal := fetch("")
	if len(normal) <= len(dates) {
		t.Fatalf("normal dates = %v (%d), want more than trainer dates (%d)", normal, len(normal), len(dates))
	}
}
