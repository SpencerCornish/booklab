package api

import (
	"time"

	"github.com/spencercornish/booklab/internal/db"
)

func dateOnly(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func dateInClosureRange(d time.Time, c *db.Closure, loc *time.Location) bool {
	from := dateOnly(c.StartDate, loc)
	to := dateOnly(c.EndDate, loc)
	return !d.Before(from) && !d.After(to)
}

func allDayClosureForDate(closures []*db.Closure, day time.Time, loc *time.Location) (bool, string) {
	d := dateOnly(day, loc)
	for _, c := range closures {
		if !c.AllDay {
			continue
		}
		if dateInClosureRange(d, c, loc) {
			reason := ""
			if c.Reason != nil {
				reason = *c.Reason
			}
			return true, reason
		}
	}
	return false, ""
}

func slotOverlapsPartialClosure(slotStart, slotEnd time.Time, c *db.Closure, loc *time.Location) bool {
	if c.AllDay || c.StartTime == nil || c.EndTime == nil {
		return false
	}
	d := slotStart.In(loc)
	sh, sm, _ := c.StartTime.Clock()
	eh, em, _ := c.EndTime.Clock()
	cStart := time.Date(d.Year(), d.Month(), d.Day(), sh, sm, 0, 0, loc)
	cEnd := time.Date(d.Year(), d.Month(), d.Day(), eh, em, 0, 0, loc)
	return slotStart.Before(cEnd) && slotEnd.After(cStart)
}

func bookingOverlapsClosure(bookingStart, bookingEnd time.Time, c *db.Closure, loc *time.Location) bool {
	bs := bookingStart.In(loc)
	be := bookingEnd.In(loc)
	if !be.After(bs) {
		return false
	}

	bookFrom := dateOnly(bs, loc)
	bookTo := dateOnly(be.Add(-time.Nanosecond), loc)

	closureFrom := dateOnly(c.StartDate, loc)
	closureTo := dateOnly(c.EndDate, loc)

	overlapStart := bookFrom
	if closureFrom.After(overlapStart) {
		overlapStart = closureFrom
	}
	overlapEnd := bookTo
	if closureTo.Before(overlapEnd) {
		overlapEnd = closureTo
	}
	if overlapStart.After(overlapEnd) {
		return false
	}

	if c.AllDay {
		return true
	}
	if c.StartTime == nil || c.EndTime == nil {
		return false
	}

	sh, sm, _ := c.StartTime.Clock()
	eh, em, _ := c.EndTime.Clock()

	for d := overlapStart; !d.After(overlapEnd); d = d.AddDate(0, 0, 1) {
		cStart := time.Date(d.Year(), d.Month(), d.Day(), sh, sm, 0, 0, loc)
		cEnd := time.Date(d.Year(), d.Month(), d.Day(), eh, em, 0, 0, loc)
		if bs.Before(cEnd) && be.After(cStart) {
			return true
		}
	}
	return false
}
