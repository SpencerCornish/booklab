package api

import (
	"context"
	"log"
	"net/http"
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
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"resource_name":      settings.ResourceName,
		"hourly_rate_cents":  settings.HourlyRateCents,
		"currency":           settings.Currency,
		"bookable_start":     settings.BookableStart.Format("15:04"),
		"bookable_end":       settings.BookableEnd.Format("15:04"),
		"min_hours":          settings.MinHours,
		"max_hours":          settings.MaxHours,
		"timezone":           settings.Timezone,
	})
}

func (s *Server) handleGetAvailability(w http.ResponseWriter, r *http.Request) {
	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		writeError(w, http.StatusBadRequest, "date is required (YYYY-MM-DD)")
		return
	}

	settings, err := s.queries.GetSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}

	loc, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		loc = time.UTC
	}

	date, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid date format, use YYYY-MM-DD")
		return
	}

	// Check if the date falls in a closure
	closures, err := s.queries.ListClosuresInRange(r.Context(), date, date.AddDate(0, 0, 1))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load closures")
		return
	}

	if len(closures) > 0 {
		reason := ""
		if closures[0].Reason != nil {
			reason = *closures[0].Reason
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"date":             dateStr,
			"is_closed":        true,
			"closure_reason":   reason,
			"slots":            []any{},
		})
		return
	}

	// Build slot list based on bookable hours
	startHour := settings.BookableStart.Hour()
	startMin := settings.BookableStart.Minute()
	endHour := settings.BookableEnd.Hour()
	endMin := settings.BookableEnd.Minute()

	dayStart := time.Date(date.Year(), date.Month(), date.Day(), startHour, startMin, 0, 0, loc)
	dayEnd := time.Date(date.Year(), date.Month(), date.Day(), endHour, endMin, 0, 0, loc)

	// Get existing bookings for this day
	bookings, err := s.queries.ListBookingsInRange(r.Context(), dayStart, dayEnd.Add(time.Hour))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load bookings")
		return
	}

	type slot struct {
		Start     time.Time `json:"start"`
		End       time.Time `json:"end"`
		Available bool      `json:"available"`
	}

	var slots []slot
	for t := dayStart; t.Before(dayEnd); t = t.Add(time.Hour) {
		slotEnd := t.Add(time.Hour)
		available := true
		for _, b := range bookings {
			// Overlap check: slot overlaps booking if slot.start < b.end && slot.end > b.start
			if t.Before(b.EndTime) && slotEnd.After(b.StartTime) {
				available = false
				break
			}
		}
		// Past slots are not available
		if t.Before(time.Now()) {
			available = false
		}
		slots = append(slots, slot{Start: t, End: slotEnd, Available: available})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"date":      dateStr,
		"is_closed": false,
		"slots":     slots,
	})
}

func (s *Server) handleCreateBooking(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string    `json:"name"`
		Email     string    `json:"email"`
		StartTime time.Time `json:"start_time"`
		EndTime   time.Time `json:"end_time"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Email == "" {
		writeError(w, http.StatusBadRequest, "name and email are required")
		return
	}
	if req.EndTime.Before(req.StartTime) || req.EndTime.Equal(req.StartTime) {
		writeError(w, http.StatusBadRequest, "end_time must be after start_time")
		return
	}

	settings, err := s.queries.GetSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}

	// Create Stripe SetupIntent
	setupIntentID, clientSecret, err := s.stripe.CreateSetupIntent(req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create payment setup")
		return
	}

	booking, err := s.queries.CreateBooking(r.Context(), req.Name, req.Email, req.StartTime, req.EndTime, setupIntentID)
	if err != nil {
		// Check for exclusion constraint violation (conflict)
		if isConflictError(err) {
			writeError(w, http.StatusConflict, "this time slot is no longer available")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create booking")
		return
	}

	// Send confirmation email + staff notification
	go func() {
		data := emailsvc.ConfirmationData{
			ResourceName: settings.ResourceName,
			BookerName:   booking.Name,
			StartTime:    booking.StartTime,
			EndTime:      booking.EndTime,
			CancelURL:    s.cfg.AppURL + "/cancel/" + booking.CancelToken.String(),
			ViewURL:      s.cfg.AppURL + "/booking/" + booking.CancelToken.String(),
		}
		if err := s.email.SendConfirmation(booking.Email, data); err != nil {
			_ = err
		}

		if settings.NotificationEmails != "" {
			priorCount, _ := s.queries.CountPriorBookings(context.Background(), booking.Email, booking.ID)
			staffData := emailsvc.StaffNewBookingData{
				ResourceName:      settings.ResourceName,
				BookerName:        booking.Name,
				BookerEmail:       booking.Email,
				StartTime:         booking.StartTime,
				EndTime:           booking.EndTime,
				IsReturnCustomer:  priorCount > 0,
				PriorBookingCount: priorCount,
				AdminURL:          s.cfg.AppURL + "/admin/bookings",
			}
			for _, addr := range splitEmails(settings.NotificationEmails) {
				if err := s.email.SendStaffNewBooking(addr, staffData); err != nil {
					log.Printf("staff new booking email to %s: %v", addr, err)
				}
			}
		}
	}()

	writeJSON(w, http.StatusCreated, map[string]any{
		"booking":                    bookingToPublic(booking),
		"setup_intent_client_secret": clientSecret,
	})
}

func (s *Server) handleGetBooking(w http.ResponseWriter, r *http.Request) {
	token, err := uuid.Parse(chi.URLParam(r, "token"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid token")
		return
	}
	booking, err := s.queries.GetBookingByToken(r.Context(), token)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "booking not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load booking")
		return
	}
	writeJSON(w, http.StatusOK, bookingToPublic(booking))
}

func (s *Server) handleConfirmBookingCard(w http.ResponseWriter, r *http.Request) {
	token, err := uuid.Parse(chi.URLParam(r, "token"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid token")
		return
	}

	booking, err := s.queries.GetBookingByToken(r.Context(), token)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "booking not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load booking")
		return
	}

	if booking.StripePaymentMethodID != nil {
		// Already stored, idempotent
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if booking.StripeSetupIntentID == nil {
		writeError(w, http.StatusBadRequest, "no setup intent on booking")
		return
	}

	pmID, err := s.stripe.GetPaymentMethodFromSetupIntent(*booking.StripeSetupIntentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "card not yet confirmed")
		return
	}

	if _, err := s.queries.UpdateBookingPaymentMethod(r.Context(), booking.ID, pmID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save payment method")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCancelBooking(w http.ResponseWriter, r *http.Request) {
	token, err := uuid.Parse(chi.URLParam(r, "token"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid token")
		return
	}

	booking, err := s.queries.CancelBooking(r.Context(), token)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusConflict, "booking not found or cannot be cancelled")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to cancel booking")
		return
	}

	settings, err := s.queries.GetSettings(r.Context())
	if err == nil {
		go func() {
			data := emailsvc.CancellationData{
				ResourceName: settings.ResourceName,
				BookerName:   booking.Name,
				StartTime:    booking.StartTime,
				EndTime:      booking.EndTime,
			}
			_ = s.email.SendCancellation(booking.Email, data)
		}()
	}

	// Detach payment method if present
	if booking.StripePaymentMethodID != nil {
		go func() { _ = s.stripe.DetachPaymentMethod(*booking.StripePaymentMethodID) }()
	}

	writeJSON(w, http.StatusOK, bookingToPublic(booking))
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
	return m
}
