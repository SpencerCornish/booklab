package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/spencercornish/booklab/internal/db"
	emailsvc "github.com/spencercornish/booklab/internal/email"
)

func (s *Server) handleGetPublicSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.queries.GetSettings(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load settings", err)
		return
	}
	// The restriction is only advertised while its end date has not passed, so the
	// booking page stops showing the notice once the window is over.
	var restriction any
	if _, windowEnd, ok := newCustomerWindow(settings, settingsLocation(settings)); ok && time.Now().Before(windowEnd) {
		restriction = map[string]any{
			"start_date": settings.NewCustomerBlockStart.Format("2006-01-02"),
			"end_date":   settings.NewCustomerBlockEnd.Format("2006-01-02"),
			"message":    newCustomerMessage(settings),
		}
	}

	// Only advertise the question; the trainer schedule stays server-side.
	var gate any
	if settings.NewUserGateEnabled {
		gate = map[string]any{
			"enabled":  true,
			"question": newUserGateQuestion(settings),
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"resource_name":            settings.ResourceName,
		"hourly_rate_cents":        settings.HourlyRateCents,
		"currency":                 settings.Currency,
		"bookable_start":           settings.BookableStart.Format("15:04"),
		"bookable_end":             settings.BookableEnd.Format("15:04"),
		"min_hours":                settings.MinHours,
		"max_hours":                settings.MaxHours,
		"min_lead_hours":           settings.MinLeadHours,
		"timezone":                 settings.Timezone,
		"referral_sources":         settings.ReferralSources,
		"new_customer_restriction": restriction,
		"new_user_gate":            gate,
	})
}

func (s *Server) handleGetTerms(w http.ResponseWriter, r *http.Request) {
	settings, err := s.queries.GetSettings(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load terms", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"content": settings.TermsContent,
	})
}

func (s *Server) handleGetPrivacy(w http.ResponseWriter, r *http.Request) {
	settings, err := s.queries.GetSettings(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load privacy policy", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"content": settings.PrivacyContent,
	})
}

func (s *Server) handleGetAvailability(w http.ResponseWriter, r *http.Request) {
	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		s.writeError(w, r, http.StatusBadRequest, "date is required (YYYY-MM-DD)", nil)
		return
	}

	settings, err := s.queries.GetSettings(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load settings", err)
		return
	}

	loc, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		loc = time.UTC
	}

	date, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid date format, use YYYY-MM-DD", err)
		return
	}

	// Check if the date falls in a closure
	closures, err := s.queries.ListClosuresInRange(r.Context(), date, date.AddDate(0, 0, 1))
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load closures", err)
		return
	}

	if closed, reason := allDayClosureForDate(closures, date, loc); closed {
		writeJSON(w, http.StatusOK, map[string]any{
			"date":           dateStr,
			"is_closed":      true,
			"closure_reason": reason,
			"slots":          []any{},
		})
		return
	}

	// The gate replaces normal hours with the trainer's weekly schedule for
	// self-reported new customers.
	newUser, parseErr := newUserQueryParam(r)
	if parseErr != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid new_user value", parseErr)
		return
	}

	var windows []bookableWindow
	if settings.NewUserGateEnabled && newUser {
		windows = newUserWindowsForDate(settings, date, loc)
	} else {
		start, end := bookableBoundsForDate(date, settings, loc)
		windows = []bookableWindow{{Start: start, End: end}}
	}
	if len(windows) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"date":      dateStr,
			"is_closed": false,
			"slots":     []any{},
		})
		return
	}

	// Fetch every booking that could overlap any window on this date.
	fetchStart, fetchEnd := windows[0].Start, windows[0].End
	for _, w := range windows[1:] {
		if w.Start.Before(fetchStart) {
			fetchStart = w.Start
		}
		if w.End.After(fetchEnd) {
			fetchEnd = w.End
		}
	}
	bookings, err := s.queries.ListBookingsOverlapping(r.Context(), fetchStart, fetchEnd)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load bookings", err)
		return
	}

	type slot struct {
		Start     time.Time `json:"start"`
		End       time.Time `json:"end"`
		Available bool      `json:"available"`
	}

	earliest := earliestBookableStart(settings)
	slots := make([]slot, 0)
	for _, w := range windows {
		for t := w.Start; !t.Add(time.Hour).After(w.End); t = t.Add(time.Hour) {
			slotEnd := t.Add(time.Hour)
			available := true
			for _, b := range bookings {
				// Overlap check: slot overlaps booking if slot.start < b.end && slot.end > b.start
				if t.Before(b.EndTime) && slotEnd.After(b.StartTime) {
					available = false
					break
				}
			}
			if available {
				for _, c := range closures {
					if slotOverlapsPartialClosure(t, slotEnd, c, loc) {
						available = false
						break
					}
				}
			}
			// Past slots, and slots inside the minimum notice period, are not available
			if t.Before(earliest) {
				available = false
			}
			slots = append(slots, slot{Start: t, End: slotEnd, Available: available})
		}
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].Start.Before(slots[j].Start) })

	writeJSON(w, http.StatusOK, map[string]any{
		"date":      dateStr,
		"is_closed": false,
		"slots":     slots,
	})
}

// handleGetAvailableDays lists the dates in the coming days that have at least
// one bookable slot, so the booking page can grey out dead days up front
// instead of making visitors click through empty dates.
func (s *Server) handleGetAvailableDays(w http.ResponseWriter, r *http.Request) {
	days := 60
	if raw := r.URL.Query().Get("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			s.writeError(w, r, http.StatusBadRequest, "invalid days value", nil)
			return
		}
		days = min(parsed, 90)
	}

	settings, err := s.queries.GetSettings(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load settings", err)
		return
	}
	loc := settingsLocation(settings)

	newUser, err := newUserQueryParam(r)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid new_user value", err)
		return
	}

	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, days)

	closures, err := s.queries.ListClosuresInRange(r.Context(), start, end)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load closures", err)
		return
	}
	bookings, err := s.queries.ListBookingsOverlapping(r.Context(), start, end)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load bookings", err)
		return
	}

	// Same routing rule as the per-date endpoint: trainer hours replace normal
	// hours only while the gate is enabled and the customer is self-reported new.
	trainerHours := settings.NewUserGateEnabled && newUser

	writeJSON(w, http.StatusOK, map[string]any{
		"available_dates": availableDates(settings, closures, bookings, start, days, loc, trainerHours),
	})
}

func (s *Server) handlePrepareBooking(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email     string    `json:"email"`
		StartTime time.Time `json:"start_time"`
		EndTime   time.Time `json:"end_time"`
		NewUser   *bool     `json:"new_user"`
	}
	if !s.readJSONRequest(w, r, &req) {
		return
	}
	if req.Email == "" {
		s.writeError(w, r, http.StatusBadRequest, "email is required", nil)
		return
	}
	if req.EndTime.Before(req.StartTime) || req.EndTime.Equal(req.StartTime) {
		s.writeError(w, r, http.StatusBadRequest, "end_time must be after start_time", nil)
		return
	}

	settings, err := s.queries.GetSettings(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load settings", err)
		return
	}

	newUser, badReq := resolveNewUserAnswer(settings, req.NewUser)
	if badReq != "" {
		s.writeError(w, r, http.StatusBadRequest, badReq, nil)
		return
	}

	if badReq, err := s.validateBookingCreate(r.Context(), req.StartTime, req.EndTime, settings, newUser != nil && *newUser); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to validate booking", err)
		return
	} else if badReq != "" {
		s.writeError(w, r, http.StatusBadRequest, badReq, nil)
		return
	}

	if blocked, err := s.checkNewCustomerRestriction(r.Context(), req.Email, req.StartTime, req.EndTime, settings); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to check booking restriction", err)
		return
	} else if blocked != "" {
		s.writeError(w, r, http.StatusForbidden, blocked, nil)
		return
	}

	if err := s.ensureSlotAvailable(r.Context(), req.StartTime, req.EndTime); err != nil {
		if isConflictError(err) {
			s.writeError(w, r, http.StatusConflict, "this time slot is no longer available", err)
			return
		}
		s.writeError(w, r, http.StatusInternalServerError, "failed to check availability", err)
		return
	}

	setupIntentID, clientSecret, err := s.stripe.CreateSetupIntent(req.Email)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to create payment setup", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"setup_intent_id":            setupIntentID,
		"setup_intent_client_secret": clientSecret,
	})
}

func (s *Server) handleCreateBooking(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SetupIntentID string            `json:"setup_intent_id"`
		Name          string            `json:"name"`
		Email         string            `json:"email"`
		Metadata      map[string]string `json:"metadata"`
		StartTime     time.Time         `json:"start_time"`
		EndTime       time.Time         `json:"end_time"`
		NewUser       *bool             `json:"new_user"`
	}
	if !s.readJSONRequest(w, r, &req) {
		return
	}
	if req.SetupIntentID == "" || req.Name == "" || req.Email == "" {
		s.writeError(w, r, http.StatusBadRequest, "setup_intent_id, name, and email are required", nil)
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	if req.EndTime.Before(req.StartTime) || req.EndTime.Equal(req.StartTime) {
		s.writeError(w, r, http.StatusBadRequest, "end_time must be after start_time", nil)
		return
	}

	existing, err := s.queries.GetBookingBySetupIntentID(r.Context(), req.SetupIntentID)
	if err == nil {
		writeJSON(w, http.StatusCreated, map[string]any{"booking": bookingToPublic(existing)})
		return
	}
	if err != pgx.ErrNoRows {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load booking", err)
		return
	}

	pmID, err := s.stripe.GetPaymentMethodFromSetupIntent(req.SetupIntentID)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "card not confirmed", err)
		return
	}

	settings, err := s.queries.GetSettings(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load settings", err)
		return
	}

	newUser, badReq := resolveNewUserAnswer(settings, req.NewUser)
	if badReq != "" {
		s.writeError(w, r, http.StatusBadRequest, badReq, nil)
		return
	}

	if badReq, err := s.validateBookingCreate(r.Context(), req.StartTime, req.EndTime, settings, newUser != nil && *newUser); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to validate booking", err)
		return
	} else if badReq != "" {
		s.writeError(w, r, http.StatusBadRequest, badReq, nil)
		return
	}

	if blocked, err := s.checkNewCustomerRestriction(r.Context(), req.Email, req.StartTime, req.EndTime, settings); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to check booking restriction", err)
		return
	} else if blocked != "" {
		s.writeError(w, r, http.StatusForbidden, blocked, nil)
		return
	}

	booking, err := s.queries.CreateBooking(r.Context(), req.Name, req.Email, req.Metadata, req.StartTime, req.EndTime, req.SetupIntentID, pmID, newUser)
	if err != nil {
		if isConflictError(err) {
			s.writeError(w, r, http.StatusConflict, "this time slot is no longer available", err)
			return
		}
		s.writeError(w, r, http.StatusInternalServerError, "failed to create booking", err)
		return
	}

	if settings.ReminderHoursBefore > 0 &&
		booking.StartTime.Sub(booking.CreatedAt) < time.Duration(settings.ReminderHoursBefore)*time.Hour {
		if err := s.queries.MarkReminderSent(r.Context(), booking.ID); err != nil {
			s.logger.Error("mark short-notice booking reminder skipped", "booking_id", booking.ID, "error", err)
		} else {
			booking.ReminderSent = true
		}
	}

	s.sendBookingConfirmationEmails(booking, settings)

	writeJSON(w, http.StatusCreated, map[string]any{"booking": bookingToPublic(booking)})
}

func (s *Server) handleGetBooking(w http.ResponseWriter, r *http.Request) {
	token, err := uuid.Parse(chi.URLParam(r, "token"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid token", err)
		return
	}
	booking, err := s.queries.GetBookingByToken(r.Context(), token)
	if err != nil {
		if err == pgx.ErrNoRows {
			s.writeError(w, r, http.StatusNotFound, "booking not found", err)
			return
		}
		s.writeError(w, r, http.StatusInternalServerError, "failed to load booking", err)
		return
	}
	writeJSON(w, http.StatusOK, bookingToPublic(booking))
}

func (s *Server) sendBookingConfirmationEmails(booking *db.Booking, settings *db.Settings) {
	go func() {
		data := emailsvc.ConfirmationData{
			ResourceName: settings.ResourceName,
			BookerName:   booking.Name,
			StartTime:    booking.StartTime,
			EndTime:      booking.EndTime,
			CancelURL:    s.cfg.AppURL + "/cancel/" + booking.CancelToken.String(),
			ViewURL:      s.cfg.AppURL + "/booking/" + booking.CancelToken.String(),
			Timezone:     settings.Timezone,
		}
		if err := s.email.SendConfirmation(booking.Email, data); err != nil {
			s.logger.Error("send confirmation email failed", "booking_id", booking.ID, "email", booking.Email, "error", err)
		}
		if settings.NotificationEmails == "" {
			return
		}
		priorCount, _ := s.queries.CountPriorBookings(context.Background(), booking.Email, booking.ID)
		newUserLabel := ""
		if booking.IsNewUser != nil {
			if *booking.IsNewUser {
				newUserLabel = "New user (self-reported)"
			} else {
				newUserLabel = "Returning customer (self-reported)"
			}
		}
		staffData := emailsvc.StaffNewBookingData{
			ResourceName:      settings.ResourceName,
			BookerName:        booking.Name,
			BookerEmail:       booking.Email,
			Metadata:          booking.Metadata,
			StartTime:         booking.StartTime,
			EndTime:           booking.EndTime,
			IsReturnCustomer:  priorCount > 0,
			PriorBookingCount: priorCount,
			NewUserLabel:      newUserLabel,
			AdminURL:          s.cfg.AppURL + "/admin/bookings",
			Timezone:          settings.Timezone,
		}
		for _, addr := range splitEmails(settings.NotificationEmails) {
			if err := s.email.SendStaffNewBooking(addr, staffData); err != nil {
				s.logger.Error("staff new booking email failed", "booking_id", booking.ID, "recipient", addr, "error", err)
			}
		}
	}()
}

// ensureSlotAvailable returns an error if any non-cancelled booking overlaps [start, end).
func (s *Server) ensureSlotAvailable(ctx context.Context, start, end time.Time) error {
	bookings, err := s.queries.ListBookingsOverlapping(ctx, start, end)
	if err != nil {
		return err
	}
	if len(bookings) > 0 {
		return fmt.Errorf("bookings_no_overlap: slot conflict with booking %d", bookings[0].ID)
	}
	return nil
}

func (s *Server) handleCancelBooking(w http.ResponseWriter, r *http.Request) {
	token, err := uuid.Parse(chi.URLParam(r, "token"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid token", err)
		return
	}

	booking, err := s.queries.CancelBooking(r.Context(), token)
	if err != nil {
		if err == pgx.ErrNoRows {
			s.writeError(w, r, http.StatusConflict, "booking not found or cannot be cancelled", err)
			return
		}
		s.writeError(w, r, http.StatusInternalServerError, "failed to cancel booking", err)
		return
	}

	// Only send cancellation emails for confirmed bookings (payment method on file).
	// If there's no payment method, this is a cleanup after a failed card setup — no emails.
	if booking.StripePaymentMethodID != nil {
		settings, err := s.queries.GetSettings(r.Context())
		if err == nil {
			go func() {
				data := emailsvc.CancellationData{
					ResourceName: settings.ResourceName,
					BookerName:   booking.Name,
					StartTime:    booking.StartTime,
					EndTime:      booking.EndTime,
					Timezone:     settings.Timezone,
				}
				if err := s.email.SendCancellation(booking.Email, data); err != nil {
					s.logger.Error("send cancellation email failed", "booking_id", booking.ID, "email", booking.Email, "error", err)
				}
			}()

			if settings.NotificationEmails != "" {
				staffData := emailsvc.StaffCancellationData{
					ResourceName: settings.ResourceName,
					BookerName:   booking.Name,
					BookerEmail:  booking.Email,
					Metadata:     booking.Metadata,
					StartTime:    booking.StartTime,
					EndTime:      booking.EndTime,
					AdminURL:     s.cfg.AppURL + "/admin/bookings",
					Timezone:     settings.Timezone,
				}
				for _, addr := range splitEmails(settings.NotificationEmails) {
					addr := addr
					go func() {
						if err := s.email.SendStaffCancellation(addr, staffData); err != nil {
							s.logger.Error("staff cancellation email failed", "booking_id", booking.ID, "recipient", addr, "error", err)
						}
					}()
				}
			}
		}
	}

	// Detach payment method if present
	if booking.StripePaymentMethodID != nil {
		go func() {
			if err := s.stripe.DetachPaymentMethod(*booking.StripePaymentMethodID); err != nil {
				s.logger.Error("stripe detach on cancel failed", "booking_id", booking.ID, "payment_method_id", *booking.StripePaymentMethodID, "error", err)
			}
		}()
	}

	writeJSON(w, http.StatusOK, bookingToPublic(booking))
}

func bookableBoundsForDate(day time.Time, st *db.Settings, loc *time.Location) (windowStart, windowEnd time.Time) {
	d := day.In(loc)
	y, m, dd := d.Date()
	sh, sm, _ := st.BookableStart.Clock()
	eh, em, _ := st.BookableEnd.Clock()
	windowStart = time.Date(y, m, dd, sh, sm, 0, 0, loc)
	windowEnd = time.Date(y, m, dd, eh, em, 0, 0, loc)
	return windowStart, windowEnd
}

// bookingWithinBookableHours reports whether [start, end) lies within configured
// bookable hours on every local calendar day the interval touches.
func bookingWithinBookableHours(start, end time.Time, st *db.Settings, loc *time.Location) bool {
	start = start.In(loc)
	end = end.In(loc)
	if !end.After(start) {
		return false
	}
	for cur := start; cur.Before(end); {
		y, m, d := cur.Date()
		day := time.Date(y, m, d, 0, 0, 0, 0, loc)
		nextMidnight := day.AddDate(0, 0, 1)
		ws, we := bookableBoundsForDate(day, st, loc)
		segEnd := end
		if segEnd.After(nextMidnight) {
			segEnd = nextMidnight
		}
		if cur.Before(ws) || segEnd.After(we) {
			return false
		}
		cur = nextMidnight
	}
	return true
}

// bookableWindow is one contiguous range of bookable time on a single day.
type bookableWindow struct {
	Start time.Time
	End   time.Time
}

// parseClock parses an HH:MM clock value.
func parseClock(v string) (time.Time, bool) {
	t, err := time.Parse("15:04", v)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// newUserWindowsForDate returns the merged trainer windows for the date's
// weekday, expressed in loc. Entries with invalid or inverted times are
// skipped rather than blocking availability.
func newUserWindowsForDate(settings *db.Settings, date time.Time, loc *time.Location) []bookableWindow {
	d := date.In(loc)
	y, m, day := d.Date()
	weekday := int(d.Weekday())

	var windows []bookableWindow
	for _, w := range settings.NewUserGateSchedule {
		if w.Weekday != weekday {
			continue
		}
		start, okStart := parseClock(w.Start)
		end, okEnd := parseClock(w.End)
		if !okStart || !okEnd || !end.After(start) {
			continue
		}
		windows = append(windows, bookableWindow{
			Start: time.Date(y, m, day, start.Hour(), start.Minute(), 0, 0, loc),
			End:   time.Date(y, m, day, end.Hour(), end.Minute(), 0, 0, loc),
		})
	}
	return mergeBookableWindows(windows)
}

// mergeBookableWindows sorts windows by start and merges overlapping or
// adjacent ones so selections can span window boundaries.
func mergeBookableWindows(windows []bookableWindow) []bookableWindow {
	if len(windows) == 0 {
		return nil
	}
	sort.Slice(windows, func(i, j int) bool { return windows[i].Start.Before(windows[j].Start) })
	merged := []bookableWindow{windows[0]}
	for _, w := range windows[1:] {
		last := &merged[len(merged)-1]
		if !w.Start.After(last.End) {
			if w.End.After(last.End) {
				last.End = w.End
			}
			continue
		}
		merged = append(merged, w)
	}
	return merged
}

// bookingWithinNewUserWindows reports whether [start, end) is covered by the
// trainer windows on every local calendar day the interval touches.
func bookingWithinNewUserWindows(start, end time.Time, settings *db.Settings, loc *time.Location) bool {
	start = start.In(loc)
	end = end.In(loc)
	if !end.After(start) {
		return false
	}
	for cur := start; cur.Before(end); {
		y, m, d := cur.Date()
		day := time.Date(y, m, d, 0, 0, 0, 0, loc)
		nextMidnight := day.AddDate(0, 0, 1)
		segEnd := end
		if segEnd.After(nextMidnight) {
			segEnd = nextMidnight
		}
		if !windowCovers(newUserWindowsForDate(settings, day, loc), cur, segEnd) {
			return false
		}
		cur = nextMidnight
	}
	return true
}

// windowCovers reports whether some window fully contains [start, end).
func windowCovers(windows []bookableWindow, start, end time.Time) bool {
	for _, w := range windows {
		if !start.Before(w.Start) && !end.After(w.End) {
			return true
		}
	}
	return false
}

// availableDates returns the YYYY-MM-DD dates in the days-long window starting
// at start (local midnight) that offer at least minHours of contiguous
// bookable time, honouring closures, existing bookings, and minimum notice.
// trainerHours selects the new-user gate's weekly schedule instead of the
// normal bookable hours.
func availableDates(settings *db.Settings, closures []*db.Closure, bookings []*db.Booking, start time.Time, days int, loc *time.Location, trainerHours bool) []string {
	minHours := max(int(settings.MinHours), 1)
	earliest := earliestBookableStart(settings)

	dates := make([]string, 0, days)
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i)
		if closed, _ := allDayClosureForDate(closures, day, loc); closed {
			continue
		}
		var windows []bookableWindow
		if trainerHours {
			windows = newUserWindowsForDate(settings, day, loc)
		} else {
			windowStart, windowEnd := bookableBoundsForDate(day, settings, loc)
			windows = []bookableWindow{{Start: windowStart, End: windowEnd}}
		}
		if dayHasBookableRun(windows, bookings, closures, earliest, minHours, loc) {
			dates = append(dates, day.Format("2006-01-02"))
		}
	}
	return dates
}

// dayHasBookableRun reports whether any window contains minHours consecutive
// available hourly slots. Runs break at window gaps, matching the picker's
// contiguity rule, so a day only counts when a valid selection exists.
func dayHasBookableRun(windows []bookableWindow, bookings []*db.Booking, closures []*db.Closure, earliest time.Time, minHours int, loc *time.Location) bool {
	for _, w := range windows {
		run := 0
		for t := w.Start; !t.Add(time.Hour).After(w.End); t = t.Add(time.Hour) {
			slotEnd := t.Add(time.Hour)
			available := !t.Before(earliest)
			if available {
				for _, b := range bookings {
					// Overlap check: slot overlaps booking if slot.start < b.end && slot.end > b.start
					if t.Before(b.EndTime) && slotEnd.After(b.StartTime) {
						available = false
						break
					}
				}
			}
			if available {
				for _, c := range closures {
					if slotOverlapsPartialClosure(t, slotEnd, c, loc) {
						available = false
						break
					}
				}
			}
			if available {
				run++
				if run >= minHours {
					return true
				}
			} else {
				run = 0
			}
		}
	}
	return false
}

// earliestBookableStart is the earliest start time a new booking may have,
// given the minimum notice period in settings.
func earliestBookableStart(settings *db.Settings) time.Time {
	return time.Now().Add(time.Duration(settings.MinLeadHours) * time.Hour)
}

func (s *Server) validateBookingCreate(ctx context.Context, start, end time.Time, settings *db.Settings, newUser bool) (badRequest string, err error) {
	if start.Before(time.Now()) {
		return "booking start time is in the past", nil
	}
	if start.Before(earliestBookableStart(settings)) {
		unit := "hours"
		if settings.MinLeadHours == 1 {
			unit = "hour"
		}
		return fmt.Sprintf("bookings must be made at least %d %s in advance", settings.MinLeadHours, unit), nil
	}

	loc, lerr := time.LoadLocation(settings.Timezone)
	if lerr != nil {
		loc = time.UTC
	}

	dur := end.Sub(start)
	minDur := time.Duration(settings.MinHours) * time.Hour
	maxDur := time.Duration(settings.MaxHours) * time.Hour
	if dur < minDur {
		return "booking is shorter than the minimum allowed duration", nil
	}
	if dur > maxDur {
		return "booking exceeds the maximum allowed duration", nil
	}
	if newUser {
		// The gate replaces normal hours with the trainer's weekly schedule.
		if !bookingWithinNewUserWindows(start, end, settings, loc) {
			return "new customers can only book during trainer hours", nil
		}
	} else if !bookingWithinBookableHours(start, end, settings, loc) {
		return "booking falls outside allowed hours", nil
	}

	startL := start.In(loc)
	endL := end.In(loc)
	fromQ := time.Date(startL.Year(), startL.Month(), startL.Day(), 0, 0, 0, 0, loc)
	endDay := time.Date(endL.Year(), endL.Month(), endL.Day(), 0, 0, 0, 0, loc)
	toQ := endDay.AddDate(0, 0, 1)
	closures, err := s.queries.ListClosuresInRange(ctx, fromQ, toQ)
	if err != nil {
		return "", err
	}
	for _, c := range closures {
		if bookingOverlapsClosure(start, end, c, loc) {
			return "resource is closed for part of this booking", nil
		}
	}
	return "", nil
}

// settingsLocation returns the configured booking timezone, falling back to UTC.
func settingsLocation(settings *db.Settings) *time.Location {
	loc, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// newUserQueryParam parses the optional new_user query parameter (absent = false).
func newUserQueryParam(r *http.Request) (bool, error) {
	raw := r.URL.Query().Get("new_user")
	if raw == "" {
		return false, nil
	}
	return strconv.ParseBool(raw)
}

// defaultNewUserGateQuestion is used when the admin enables the gate without
// writing their own question text.
const defaultNewUserGateQuestion = "Have you used this space before?"

// newUserGateQuestion returns the admin's question, or the default when blank.
func newUserGateQuestion(settings *db.Settings) string {
	if strings.TrimSpace(settings.NewUserGateQuestion) != "" {
		return settings.NewUserGateQuestion
	}
	return defaultNewUserGateQuestion
}

// resolveNewUserAnswer validates the self-reported answer against the gate
// settings. It returns nil when the gate is disabled (nothing is stored), and
// a bad-request message when the gate is enabled but no answer was sent.
func resolveNewUserAnswer(settings *db.Settings, answer *bool) (*bool, string) {
	if !settings.NewUserGateEnabled {
		return nil, ""
	}
	if answer == nil {
		return nil, "please answer the pre-booking question"
	}
	return answer, ""
}

// newCustomerWindow returns the new-customer restriction as the half-open interval
// [start, end) in loc, covering whole days from the start date through the end date.
// ok is false when no window is configured.
func newCustomerWindow(settings *db.Settings, loc *time.Location) (start, end time.Time, ok bool) {
	if settings.NewCustomerBlockStart == nil || settings.NewCustomerBlockEnd == nil {
		return time.Time{}, time.Time{}, false
	}
	s := *settings.NewCustomerBlockStart
	e := *settings.NewCustomerBlockEnd
	start = time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, loc)
	end = time.Date(e.Year(), e.Month(), e.Day()+1, 0, 0, 0, 0, loc)
	return start, end, true
}

// newCustomerMessage returns the admin's message, or a default built from the window dates.
// Only call it when a window is configured.
func newCustomerMessage(settings *db.Settings) string {
	if settings.NewCustomerBlockMessage != "" {
		return settings.NewCustomerBlockMessage
	}
	return fmt.Sprintf("We're only accepting returning customers between %s and %s.",
		settings.NewCustomerBlockStart.Format("January 2, 2006"),
		settings.NewCustomerBlockEnd.Format("January 2, 2006"))
}

// checkNewCustomerRestriction returns the restriction message when the session overlaps
// the new-customer window and the email has no completed booking. An empty string means
// the booking may proceed.
func (s *Server) checkNewCustomerRestriction(ctx context.Context, email string, start, end time.Time, settings *db.Settings) (string, error) {
	windowStart, windowEnd, ok := newCustomerWindow(settings, settingsLocation(settings))
	if !ok || !start.Before(windowEnd) || !end.After(windowStart) {
		return "", nil
	}
	returning, err := s.queries.HasCompletedBooking(ctx, email)
	if err != nil {
		return "", err
	}
	if returning {
		return "", nil
	}
	return newCustomerMessage(settings), nil
}

// splitEmails parses a comma-separated list of email addresses.
func splitEmails(s string) []string {
	var out []string
	for _, addr := range strings.Split(s, ",") {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			out = append(out, addr)
		}
	}
	return out
}

// isConflictError checks if the error is a PostgreSQL exclusion constraint violation.
func isConflictError(err error) bool {
	return err != nil && (contains(err.Error(), "bookings_no_overlap") || contains(err.Error(), "exclusion constraint"))
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func bookingToPublic(b *db.Booking) map[string]any {
	return map[string]any{
		"id":           b.ID,
		"name":         b.Name,
		"email":        b.Email,
		"metadata":     b.Metadata,
		"start_time":   b.StartTime,
		"end_time":     b.EndTime,
		"status":       b.Status,
		"cancel_token": b.CancelToken,
		"created_at":   b.CreatedAt,
	}
}

func bookingToAdmin(b *db.Booking) map[string]any {
	m := bookingToPublic(b)
	m["stripe_setup_intent_id"] = b.StripeSetupIntentID
	m["stripe_payment_method_id"] = b.StripePaymentMethodID
	m["stripe_payment_intent_id"] = b.StripePaymentIntentID
	m["stripe_receipt_url"] = b.StripeReceiptURL
	m["amount_cents"] = b.AmountCents
	m["completed_at"] = b.CompletedAt
	m["metadata"] = b.Metadata
	m["reminder_sent"] = b.ReminderSent
	m["updated_at"] = b.UpdatedAt
	m["charge_attempts"] = b.ChargeAttempts
	m["last_charge_error"] = b.LastChargeError
	m["is_new_user"] = b.IsNewUser
	return m
}
