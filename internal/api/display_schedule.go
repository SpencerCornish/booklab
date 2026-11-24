package api

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/spencercornish/booklab/internal/db"
	"github.com/spencercornish/booklab/internal/display"
)

// signLookaheadDays bounds how far ahead the door sign searches for the next
// day with bookable hours (long closures, e.g. a holiday break).
const signLookaheadDays = 14

// signInput is everything needed to lay out the door sign. Bookings must be
// the non-cancelled bookings and closures covering today through the
// lookahead window.
type signInput struct {
	Now          time.Time
	Settings     *db.Settings
	Loc          *time.Location
	Bookings     []*db.Booking
	Closures     []*db.Closure
	QRURL        string
	QRLabel      string
	BatteryLow   bool
	OpenInterval time.Duration // device refresh interval during bookable hours
}

type signResult struct {
	View         display.View
	RefreshAfter time.Duration
}

// signDay is one calendar day's bookable window.
type signDay struct {
	Date       time.Time // local midnight
	Start, End time.Time
	Closed     bool   // all-day closure
	Reason     string // closure reason
}

func signDayFor(date time.Time, in signInput) signDay {
	d := signDay{Date: date}
	d.Start, d.End = bookableBoundsForDate(date, in.Settings, in.Loc)
	d.Closed, d.Reason = allDayClosureForDate(in.Closures, date, in.Loc)
	return d
}

func (d signDay) hasHours() bool { return !d.Closed && d.End.After(d.Start) }

// buildSign lays out the door sign: the left panel describes right now, the
// timeline shows today until closing time and the next open day after that.
func buildSign(in signInput) signResult {
	now := in.Now.In(in.Loc)
	today := signDayFor(dateOnly(now, in.Loc), in)

	// The day the timeline shows: today while it still has hours ahead,
	// otherwise the next day with bookable hours.
	shown, found := today, today.hasHours() && now.Before(today.End)
	if !found {
		for i := 1; i <= signLookaheadDays; i++ {
			d := signDayFor(today.Date.AddDate(0, 0, i), in)
			if d.hasHours() {
				shown, found = d, true
				break
			}
		}
	}

	v := display.View{
		Title:      in.Settings.ResourceName,
		QRURL:      in.QRURL,
		QRLabel:    in.QRLabel,
		BatteryLow: in.BatteryLow,
	}

	if !found {
		v.Status, v.Headline = display.StatusClosed, "CLOSED"
		v.Detail = "no upcoming hours"
		if today.Closed && today.Reason != "" {
			v.Next = today.Reason
		}
		v.DayLabel = dayLabel(today.Date, today.Date)
		v.EmptyMessage = "No bookable hours in the next two weeks"
		return signResult{View: v, RefreshAfter: time.Hour}
	}

	v.DayLabel = dayLabel(shown.Date, today.Date)
	v.Rows = signRows(shown, now, in)

	open := shown.Date.Equal(today.Date) && !now.Before(today.Start)
	if open {
		describeOpenHours(&v, now, today, in)
	} else {
		v.Status, v.Headline = display.StatusClosed, "CLOSED"
		v.Detail = "opens " + clock(shown.Start)
		v.DetailStrong = relativeDay(shown.Date, today.Date)
		if today.Closed && today.Reason != "" {
			v.Next = "Today: " + today.Reason
		}
	}

	return signResult{View: v, RefreshAfter: signRefresh(now, open, shown, in)}
}

// describeOpenHours fills the left panel while the room is within today's
// bookable hours.
func describeOpenHours(v *display.View, now time.Time, today signDay, in signInput) {
	if b := bookingAt(in.Bookings, now); b != nil {
		v.Status, v.Headline = display.StatusInUse, "IN USE"
		v.Detail = "until " + clock(b.EndTime.In(in.Loc))
		v.DetailStrong = shortName(b.Name)
		v.Next = nextOpenLine(now, b.EndTime, today, in)
		return
	}
	if c := partialClosureAt(in.Closures, now, in.Loc); c != nil {
		_, end := partialClosureBounds(c, now, in.Loc)
		// A closure may run past today's closing time; the sign should not say
		// the room is closed until 11 PM when the building shuts at 6.
		if end.After(today.End) {
			end = today.End
		}
		v.Status, v.Headline = display.StatusClosed, "CLOSED"
		v.Detail = "until " + clock(end)
		if c.Reason != nil && *c.Reason != "" {
			v.DetailStrong = *c.Reason
		}
		v.Next = nextOpenLine(now, end, today, in)
		return
	}

	v.Status, v.Headline = display.StatusOpen, "OPEN"
	freeUntil := freeUntilFrom(now, today, in)
	v.Detail = "free until " + clock(freeUntil.In(in.Loc))
	if freeUntil.Before(today.End) {
		v.Next = "Closes " + clock(today.End)
	}
}

// nextOpenLine describes when the room next frees up today, walking past
// back-to-back bookings and closures. busyUntil is the end of the current
// booking or closure, already shown in the detail line; when the room frees up
// right then, the line says how long it stays free instead of repeating it.
func nextOpenLine(now, busyUntil time.Time, today signDay, in signInput) string {
	t := now
	busy := "Booked" // what is holding the room if it stays busy until closing
	for t.Before(today.End) {
		if b := bookingAt(in.Bookings, t); b != nil {
			t = b.EndTime
			busy = "Booked"
			continue
		}
		if c := partialClosureAt(in.Closures, t, in.Loc); c != nil {
			_, t = partialClosureBounds(c, t, in.Loc)
			busy = "Closed"
			continue
		}
		if !t.Equal(busyUntil) {
			return "Next open: " + clock(t.In(in.Loc))
		}
		if free := freeUntilFrom(t, today, in); free.Before(today.End) {
			return "Then free until " + clock(free.In(in.Loc))
		}
		return "Then free until closing"
	}
	return busy + " until closing"
}

// freeUntilFrom is when the free stretch starting at t ends: the next booking
// or partial closure, or closing time.
func freeUntilFrom(t time.Time, today signDay, in signInput) time.Time {
	free := today.End
	for _, b := range in.Bookings {
		if b.StartTime.After(t) && b.StartTime.Before(free) {
			free = b.StartTime
		}
	}
	for _, c := range in.Closures {
		if s, _, ok := partialClosureOnDay(c, today.Date, in.Loc); ok && s.After(t) && s.Before(free) {
			free = s
		}
	}
	return free
}

func signRows(day signDay, now time.Time, in signInput) []display.Row {
	var rows []display.Row
	for t := day.Start; !t.Add(time.Hour).After(day.End); t = t.Add(time.Hour) {
		end := t.Add(time.Hour)
		row := display.Row{
			Label:   hourLabel(t.In(in.Loc)),
			Current: !now.Before(t) && now.Before(end),
			Past:    !end.After(now),
		}
		for _, b := range in.Bookings {
			if t.Before(b.EndTime) && end.After(b.StartTime) {
				row.State = display.RowBooked
				row.Text = shortName(b.Name)
				break
			}
		}
		if row.State == display.RowOpen {
			for _, c := range in.Closures {
				if slotOverlapsPartialClosure(t, end, c, in.Loc) {
					row.State = display.RowClosed
					break
				}
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// signRefresh picks how long the device sleeps. During bookable hours it
// wakes every OpenInterval, and also just after the next hour boundary or
// booking edge so the "now" marker and status flip on time. Outside hours it
// sleeps until shortly before opening, but at most an hour so bookings made
// overnight still appear.
func signRefresh(now time.Time, open bool, shown signDay, in signInput) time.Duration {
	const settle = 20 * time.Second // device clocks drift; land just after the edge
	interval := in.OpenInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if !open {
		// Wake hourly so bookings made overnight still appear, but land 15
		// minutes before opening (the pre-open countdown) and again at opening
		// itself, even when the configured interval is longer than that window.
		preOpen := shown.Start.Add(-15 * time.Minute)
		wake := preOpen.Sub(now)
		if !now.Before(preOpen) {
			wake = shown.Start.Sub(now) + settle
		}
		return clampDuration(wake, time.Minute, time.Hour)
	}

	// Next timeline row boundary (rows start at the opening time, which may be on the half hour).
	next := shown.Start.Add((now.Sub(shown.Start)/time.Hour + 1) * time.Hour)
	consider := func(t time.Time) {
		if t.After(now) && t.Before(next) {
			next = t
		}
	}
	consider(shown.End)
	for _, b := range in.Bookings {
		consider(b.StartTime)
		consider(b.EndTime)
	}
	for _, c := range in.Closures {
		if s, e, ok := partialClosureOnDay(c, shown.Date, in.Loc); ok {
			consider(s)
			consider(e)
		}
	}
	return clampDuration(next.Sub(now)+settle, time.Minute, interval)
}

func clampDuration(d, lo, hi time.Duration) time.Duration {
	return max(lo, min(hi, d))
}

func bookingAt(bookings []*db.Booking, t time.Time) *db.Booking {
	for _, b := range bookings {
		if !t.Before(b.StartTime) && t.Before(b.EndTime) {
			return b
		}
	}
	return nil
}

// partialClosureOnDay returns a partial closure's bounds on the given local day.
func partialClosureOnDay(c *db.Closure, day time.Time, loc *time.Location) (start, end time.Time, ok bool) {
	if c.AllDay || c.StartTime == nil || c.EndTime == nil || !dateInClosureRange(dateOnly(day, loc), c, loc) {
		return time.Time{}, time.Time{}, false
	}
	start, end = partialClosureBounds(c, day, loc)
	return start, end, true
}

func partialClosureBounds(c *db.Closure, day time.Time, loc *time.Location) (start, end time.Time) {
	d := day.In(loc)
	sh, sm, _ := c.StartTime.Clock()
	eh, em, _ := c.EndTime.Clock()
	return time.Date(d.Year(), d.Month(), d.Day(), sh, sm, 0, 0, loc),
		time.Date(d.Year(), d.Month(), d.Day(), eh, em, 0, 0, loc)
}

func partialClosureAt(closures []*db.Closure, t time.Time, loc *time.Location) *db.Closure {
	for _, c := range closures {
		if s, e, ok := partialClosureOnDay(c, t, loc); ok && !t.Before(s) && t.Before(e) {
			return c
		}
	}
	return nil
}

// shortName turns "Maya Rodriguez" into "Maya R." so the public sign never
// shows a full name. A single word is shown as-is (capitalized).
func shortName(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return ""
	}
	first := capitalize(parts[0])
	if len(parts) == 1 {
		return first
	}
	last, _ := utf8.DecodeRuneInString(parts[len(parts)-1])
	if !unicode.IsLetter(last) {
		return first
	}
	return first + " " + string(unicode.ToUpper(last)) + "."
}

func capitalize(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

func clock(t time.Time) string { return t.Format("3:04 PM") }

func hourLabel(t time.Time) string {
	if t.Minute() != 0 {
		return t.Format("3:04")
	}
	return t.Format("3 PM")
}

func dayLabel(day, today time.Time) string {
	s := strings.ToUpper(day.Format("Mon Jan 2"))
	if day.Equal(today.AddDate(0, 0, 1)) {
		return "TOMORROW · " + s
	}
	return s
}

func relativeDay(day, today time.Time) string {
	switch {
	case day.Equal(today):
		return "today"
	case day.Equal(today.AddDate(0, 0, 1)):
		return "tomorrow"
	default:
		return day.Format("Mon, Jan 2")
	}
}
