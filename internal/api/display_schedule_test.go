package api

import (
	"strings"
	"testing"
	"time"

	"github.com/spencercornish/booklab/internal/db"
	"github.com/spencercornish/booklab/internal/display"
)

// Friday 2030-11-22, Denver (behind UTC so date math is exercised).
var signLoc = mustLoc("America/Denver")

func mustLoc(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

func at(day, hour, min int) time.Time {
	return time.Date(2030, 11, day, hour, min, 0, 0, signLoc)
}

func clockOf(h, m int) time.Time { return time.Date(2000, 1, 1, h, m, 0, 0, time.UTC) }

func signSettings() *db.Settings {
	return &db.Settings{
		ResourceName:  "Darkroom",
		Timezone:      "America/Denver",
		BookableStart: clockOf(10, 0),
		BookableEnd:   clockOf(18, 0),
	}
}

func booking(name string, start, end time.Time) *db.Booking {
	return &db.Booking{Name: name, StartTime: start, EndTime: end, Status: db.BookingStatusConfirmed}
}

// storedDate mimics a DATE column scanned by pgx: UTC midnight of the calendar date.
func storedDate(day int) time.Time { return time.Date(2030, 11, day, 0, 0, 0, 0, time.UTC) }

func runSign(now time.Time, bookings []*db.Booking, closures []*db.Closure) signResult {
	return buildSign(signInput{
		Now: now, Settings: signSettings(), Loc: signLoc,
		Bookings: bookings, Closures: closures,
		OpenInterval: 5 * time.Minute,
	})
}

func TestBuildSign_inUse(t *testing.T) {
	bs := []*db.Booking{
		booking("Maya Rodriguez", at(22, 12, 0), at(22, 14, 0)),
		booking("jon paul smith", at(22, 14, 0), at(22, 15, 0)),
	}
	res := runSign(at(22, 13, 10), bs, nil)
	v := res.View

	if v.Status != display.StatusInUse || v.Headline != "IN USE" {
		t.Fatalf("status = %v %q", v.Status, v.Headline)
	}
	if v.Detail != "until 2:00 PM" || v.DetailStrong != "Maya R." {
		t.Fatalf("detail = %q / %q", v.Detail, v.DetailStrong)
	}
	// Back-to-back booking pushes the next free time to 3 PM.
	if v.Next != "Next open: 3:00 PM" {
		t.Fatalf("next = %q", v.Next)
	}
	if v.DayLabel != "FRI NOV 22" {
		t.Fatalf("day label = %q", v.DayLabel)
	}
	if len(v.Rows) != 8 {
		t.Fatalf("rows = %d, want 8", len(v.Rows))
	}
	want := []struct {
		label string
		state display.RowState
		text  string
		cur   bool
		past  bool
	}{
		{"10 AM", display.RowOpen, "", false, true},
		{"11 AM", display.RowOpen, "", false, true},
		{"12 PM", display.RowBooked, "Maya R.", false, true},
		{"1 PM", display.RowBooked, "Maya R.", true, false},
		{"2 PM", display.RowBooked, "Jon S.", false, false},
		{"3 PM", display.RowOpen, "", false, false},
	}
	for i, w := range want {
		r := v.Rows[i]
		if r.Label != w.label || r.State != w.state || r.Text != w.text || r.Current != w.cur || r.Past != w.past {
			t.Errorf("row %d = %+v, want %+v", i, r, w)
		}
	}
	// Wakes just after the 2 PM booking edge, capped at the open interval.
	if res.RefreshAfter != 5*time.Minute {
		t.Fatalf("refresh = %v", res.RefreshAfter)
	}
}

func TestBuildSign_refreshLandsOnEdges(t *testing.T) {
	bs := []*db.Booking{booking("A B", at(22, 12, 0), at(22, 14, 0))}
	res := runSign(at(22, 13, 58), bs, nil)
	if want := 2*time.Minute + 20*time.Second; res.RefreshAfter != want {
		t.Fatalf("refresh = %v, want %v", res.RefreshAfter, want)
	}
}

func TestBuildSign_open(t *testing.T) {
	bs := []*db.Booking{booking("Alex", at(22, 16, 0), at(22, 17, 0))}
	v := runSign(at(22, 11, 30), bs, nil).View
	if v.Status != display.StatusOpen || v.Detail != "free until 4:00 PM" || v.Next != "Closes 6:00 PM" {
		t.Fatalf("got %v %q %q", v.Status, v.Detail, v.Next)
	}
	if v.Rows[6].Text != "Alex" {
		t.Fatalf("single-word name should be kept: %q", v.Rows[6].Text)
	}

	v = runSign(at(22, 17, 30), bs, nil).View
	if v.Detail != "free until 6:00 PM" || v.Next != "" {
		t.Fatalf("open until close: %q %q", v.Detail, v.Next)
	}
}

func TestBuildSign_beforeOpening(t *testing.T) {
	res := runSign(at(22, 8, 0), nil, nil)
	v := res.View
	if v.Status != display.StatusClosed || v.Detail != "opens 10:00 AM" || v.DetailStrong != "today" {
		t.Fatalf("got %v %q %q", v.Status, v.Detail, v.DetailStrong)
	}
	if v.DayLabel != "FRI NOV 22" || len(v.Rows) != 8 || v.Rows[0].Past || v.Rows[0].Current {
		t.Fatalf("timeline should show today, untouched: %q %+v", v.DayLabel, v.Rows[0])
	}
	if res.RefreshAfter != time.Hour {
		t.Fatalf("refresh = %v, want 1h cap", res.RefreshAfter)
	}
	// 9:30 → wake at 9:45, 15 minutes before opening.
	if r := runSign(at(22, 9, 30), nil, nil).RefreshAfter; r != 15*time.Minute {
		t.Fatalf("refresh near opening = %v", r)
	}
}

func TestBuildSign_afterHoursShowsTomorrow(t *testing.T) {
	bs := []*db.Booking{booking("Maya Rodriguez", at(23, 10, 0), at(23, 11, 0))}
	v := runSign(at(22, 19, 0), bs, nil).View
	if v.Status != display.StatusClosed || v.Detail != "opens 10:00 AM" || v.DetailStrong != "tomorrow" {
		t.Fatalf("got %v %q %q", v.Status, v.Detail, v.DetailStrong)
	}
	if v.DayLabel != "TOMORROW · SAT NOV 23" {
		t.Fatalf("day label = %q", v.DayLabel)
	}
	if v.Rows[0].State != display.RowBooked || v.Rows[0].Past {
		t.Fatalf("tomorrow's first row = %+v", v.Rows[0])
	}
}

func TestBuildSign_closureDaySkipsAhead(t *testing.T) {
	reason := "Chemistry change"
	cs := []*db.Closure{{StartDate: storedDate(22), EndDate: storedDate(24), AllDay: true, Reason: &reason}}
	v := runSign(at(22, 12, 0), nil, cs).View
	if v.Status != display.StatusClosed || v.Detail != "opens 10:00 AM" || v.DetailStrong != "Mon, Nov 25" {
		t.Fatalf("got %v %q %q", v.Status, v.Detail, v.DetailStrong)
	}
	if v.Next != "Today: Chemistry change" || v.DayLabel != "MON NOV 25" {
		t.Fatalf("next %q day %q", v.Next, v.DayLabel)
	}
}

func TestBuildSign_noUpcomingHours(t *testing.T) {
	cs := []*db.Closure{{StartDate: storedDate(1), EndDate: time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC), AllDay: true}}
	v := runSign(at(22, 12, 0), nil, cs).View
	if v.EmptyMessage == "" || len(v.Rows) != 0 {
		t.Fatalf("expected empty timeline, got %+v", v)
	}
}

func TestBuildSign_partialClosure(t *testing.T) {
	st, et := clockOf(12, 0), clockOf(14, 0)
	reason := "Cleaning"
	cs := []*db.Closure{{StartDate: storedDate(22), EndDate: storedDate(22), StartTime: &st, EndTime: &et, Reason: &reason}}

	v := runSign(at(22, 12, 30), nil, cs).View
	if v.Status != display.StatusClosed || v.Detail != "until 2:00 PM" || v.DetailStrong != "Cleaning" || v.Next != "Then free until closing" {
		t.Fatalf("got %v %q %q %q", v.Status, v.Detail, v.DetailStrong, v.Next)
	}
	if v.Rows[2].State != display.RowClosed || v.Rows[3].State != display.RowClosed || v.Rows[4].State != display.RowOpen {
		t.Fatalf("rows = %+v", v.Rows[2:5])
	}

	v = runSign(at(22, 10, 30), nil, cs).View
	if v.Status != display.StatusOpen || v.Detail != "free until 12:00 PM" {
		t.Fatalf("open before closure: %v %q", v.Status, v.Detail)
	}
}

func TestBuildSign_thenFreeUntil(t *testing.T) {
	bs := []*db.Booking{
		booking("A B", at(22, 10, 0), at(22, 11, 0)),
		booking("C D", at(22, 13, 0), at(22, 14, 0)),
	}
	v := runSign(at(22, 10, 15), bs, nil).View
	if v.Detail != "until 11:00 AM" || v.Next != "Then free until 1:00 PM" {
		t.Fatalf("got %q / %q", v.Detail, v.Next)
	}
}

func TestBuildSign_bookedUntilClose(t *testing.T) {
	bs := []*db.Booking{booking("A B", at(22, 16, 0), at(22, 18, 0))}
	v := runSign(at(22, 16, 30), bs, nil).View
	if v.Next != "Booked until closing" {
		t.Fatalf("next = %q", v.Next)
	}
}

func TestShortName(t *testing.T) {
	cases := map[string]string{
		"Maya Rodriguez":      "Maya R.",
		"  maya   rodriguez ": "Maya R.",
		"Mary Ann Smith":      "Mary S.",
		"Cher":                "Cher",
		"":                    "",
		"Zoë Ålund":           "Zoë Å.",
		"Bob 3rd":             "Bob",
	}
	for in, want := range cases {
		if got := shortName(in); got != want {
			t.Errorf("shortName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildSign_neverShowsEmail(t *testing.T) {
	b := booking("Maya Rodriguez", at(22, 12, 0), at(22, 13, 0))
	b.Email = "maya@example.com"
	v := runSign(at(22, 12, 10), []*db.Booking{b}, nil).View
	for _, s := range []string{v.Detail, v.DetailStrong, v.Next} {
		if strings.Contains(s, "@") || strings.Contains(s, "Rodriguez") {
			t.Fatalf("personal data leaked: %q", s)
		}
	}
	for _, r := range v.Rows {
		if strings.Contains(r.Text, "Rodriguez") {
			t.Fatalf("full name leaked in row: %q", r.Text)
		}
	}
}
