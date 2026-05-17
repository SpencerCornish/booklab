package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/spencercornish/booklab/internal/config"
	"github.com/spencercornish/booklab/internal/db"
	emailsvc "github.com/spencercornish/booklab/internal/email"
)

const (
	loginRateLimitWindow = 15 * time.Minute
	loginRateLimitMax    = 10
)

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	ctx := r.Context()
	since := time.Now().Add(-loginRateLimitWindow)
	ip := clientIP(r)

	ipFails, err := s.queries.CountRecentFailedLoginAttemptsByIP(ctx, ip, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	userFails, err := s.queries.CountRecentFailedLoginAttemptsByUsername(ctx, req.Username, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	if ipFails >= loginRateLimitMax || userFails >= loginRateLimitMax {
		writeError(w, http.StatusTooManyRequests, "too many failed login attempts, try again later")
		return
	}

	user, err := s.queries.GetAdminByUsername(ctx, req.Username)
	if err != nil {
		if err == pgx.ErrNoRows {
			_ = s.queries.RecordLoginAttempt(ctx, req.Username, ip, false)
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		_ = s.queries.RecordLoginAttempt(ctx, req.Username, ip, false)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := s.queries.RecordLoginAttempt(ctx, req.Username, ip, true); err != nil {
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}

	token, err := s.newAdminSession(ctx, user.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "booklab_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(config.SessionDuration.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("booklab_session"); err == nil && cookie.Value != "" {
		_ = s.queries.DeleteAdminSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "booklab_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminListBookings(w http.ResponseWriter, r *http.Request) {
	params := db.ListBookingsParams{}

	if d := r.URL.Query().Get("date"); d != "" {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid date format")
			return
		}
		params.Date = &t
	}
	if st := r.URL.Query().Get("status"); st != "" {
		status := db.BookingStatus(st)
		params.Status = &status
	}

	bookings, err := s.queries.ListBookings(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list bookings")
		return
	}

	result := make([]map[string]any, len(bookings))
	for i, b := range bookings {
		result[i] = bookingToAdmin(b)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAdminUpdateBooking(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid booking id")
		return
	}

	var req struct {
		EndTime *time.Time       `json:"end_time"`
		Status  *db.BookingStatus `json:"status"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var booking *db.Booking
	if req.EndTime != nil {
		booking, err = s.queries.UpdateBookingEndTime(r.Context(), id, *req.EndTime)
	} else if req.Status != nil {
		booking, err = s.queries.UpdateBookingStatus(r.Context(), id, *req.Status)
	} else {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "booking not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update booking")
		return
	}

	// Send staff completion notification when a booking is marked done
	if req.Status != nil && *req.Status == db.BookingStatusCompleted {
		go func() {
			settings, err := s.queries.GetSettings(context.Background())
			if err != nil || settings.NotificationEmails == "" {
				return
			}
			hours := int32(booking.EndTime.Sub(booking.StartTime).Hours())
			if hours < 1 {
				hours = 1
			}
			autoAmount := hours * settings.HourlyRateCents
			staffData := emailsvc.StaffCompletionData{
				ResourceName:    settings.ResourceName,
				BookerName:      booking.Name,
				BookerEmail:     booking.Email,
				StartTime:       booking.StartTime,
				EndTime:         booking.EndTime,
				AutoAmountCents: autoAmount,
				AutoChargeAt:    time.Now().Add(24 * time.Hour),
				AdminURL:        s.cfg.AppURL + "/admin/bookings",
			}
			for _, addr := range splitEmails(settings.NotificationEmails) {
				if err := s.email.SendStaffCompletion(addr, staffData); err != nil {
					log.Printf("staff completion email to %s: %v", addr, err)
				}
			}
		}()
	}

	writeJSON(w, http.StatusOK, bookingToAdmin(booking))
}

// $200 is a safe max for now
const maxChargeAmountCents int32 = 20_000

func (s *Server) handleAdminChargeBooking(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid booking id")
		return
	}

	var req struct {
		AmountCents *int32 `json:"amount_cents"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	booking, err := s.queries.ClaimBookingForCharge(r.Context(), id)
	if err != nil {
		if err == pgx.ErrNoRows {
			if _, gerr := s.queries.GetBookingByID(r.Context(), id); gerr == pgx.ErrNoRows {
				writeError(w, http.StatusNotFound, "booking not found")
				return
			}
			writeError(w, http.StatusConflict, "booking cannot be charged (already charged, in progress, or not eligible)")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to claim booking for charge")
		return
	}

	settings, err := s.queries.GetSettings(r.Context())
	if err != nil {
		_ = s.queries.RevertBookingFromChargingToCompleted(r.Context(), id)
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}

	var amountCents int32
	if req.AmountCents != nil {
		if *req.AmountCents <= 0 || *req.AmountCents > maxChargeAmountCents {
			_ = s.queries.RevertBookingFromChargingToCompleted(r.Context(), id)
			writeError(w, http.StatusBadRequest, "invalid amount_cents")
			return
		}
		amountCents = *req.AmountCents
	} else {
		hours := int32(booking.EndTime.Sub(booking.StartTime).Hours())
		if hours < 1 {
			hours = 1
		}
		amountCents = hours * settings.HourlyRateCents
		if amountCents <= 0 || amountCents > maxChargeAmountCents {
			_ = s.queries.RevertBookingFromChargingToCompleted(r.Context(), id)
			writeError(w, http.StatusBadRequest, "computed charge amount is out of allowed range")
			return
		}
	}

	description := settings.ResourceName + " – " + booking.StartTime.Format("Jan 2, 2006")
	idempotencyKey := fmt.Sprintf("charge-booking-%d-admin", id)
	piID, receiptURL, err := s.stripe.ChargePaymentMethod(*booking.StripePaymentMethodID, int64(amountCents), settings.Currency, description, idempotencyKey)
	if err != nil {
		_ = s.queries.RevertBookingFromChargingToCompleted(r.Context(), id)
		log.Printf("charge booking %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "payment failed")
		return
	}

	updated, err := s.queries.UpdateBookingCharged(r.Context(), id, piID, receiptURL, amountCents)
	if err != nil {
		if err == pgx.ErrNoRows {
			log.Printf("charge booking %d: Stripe succeeded but DB finalize failed (no charging row): pi=%s", id, piID)
		}
		writeError(w, http.StatusInternalServerError, "failed to record charge")
		return
	}

	// Send receipt email
	go func() {
		data := emailsvc.ReceiptData{
			ResourceName:     settings.ResourceName,
			BookerName:       booking.Name,
			StartTime:        booking.StartTime,
			EndTime:          booking.EndTime,
			AmountCents:      amountCents,
			StripeReceiptURL: receiptURL,
		}
		_ = s.email.SendReceipt(booking.Email, data)
	}()

	writeJSON(w, http.StatusOK, bookingToAdmin(updated))
}

func (s *Server) handleAdminGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.queries.GetSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}
	writeJSON(w, http.StatusOK, settingsToMap(settings))
}

func (s *Server) handleAdminUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceName        *string `json:"resource_name"`
		HourlyRateCents     *int32  `json:"hourly_rate_cents"`
		Currency            *string `json:"currency"`
		Timezone            *string `json:"timezone"`
		BookableStart       *string `json:"bookable_start"`
		BookableEnd         *string `json:"bookable_end"`
		MinHours            *int32  `json:"min_hours"`
		MaxHours            *int32  `json:"max_hours"`
		ReminderHoursBefore *int32  `json:"reminder_hours_before"`
		NotificationEmails  *string `json:"notification_emails"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	params := db.UpdateSettingsParams{
		ResourceName:        req.ResourceName,
		HourlyRateCents:     req.HourlyRateCents,
		Currency:            req.Currency,
		Timezone:            req.Timezone,
		MinHours:            req.MinHours,
		MaxHours:            req.MaxHours,
		ReminderHoursBefore: req.ReminderHoursBefore,
		NotificationEmails:  req.NotificationEmails,
	}

	if req.BookableStart != nil {
		t, err := time.Parse("15:04", *req.BookableStart)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid bookable_start format (HH:MM)")
			return
		}
		params.BookableStart = &t
	}
	if req.BookableEnd != nil {
		t, err := time.Parse("15:04", *req.BookableEnd)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid bookable_end format (HH:MM)")
			return
		}
		params.BookableEnd = &t
	}

	settings, err := s.queries.UpdateSettings(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update settings")
		return
	}
	writeJSON(w, http.StatusOK, settingsToMap(settings))
}

func (s *Server) handleAdminListClosures(w http.ResponseWriter, r *http.Request) {
	closures, err := s.queries.ListClosures(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list closures")
		return
	}
	result := make([]map[string]any, len(closures))
	for i, c := range closures {
		result[i] = closureToMap(c)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAdminCreateClosure(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StartDate string  `json:"start_date"`
		EndDate   string  `json:"end_date"`
		Reason    *string `json:"reason"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	start, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid start_date (YYYY-MM-DD)")
		return
	}
	end, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid end_date (YYYY-MM-DD)")
		return
	}

	closure, err := s.queries.CreateClosure(r.Context(), start, end, req.Reason)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create closure")
		return
	}
	writeJSON(w, http.StatusCreated, closureToMap(closure))
}

func (s *Server) handleAdminUpdateClosure(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid closure id")
		return
	}

	var req struct {
		StartDate string  `json:"start_date"`
		EndDate   string  `json:"end_date"`
		Reason    *string `json:"reason"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	start, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid start_date")
		return
	}
	end, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid end_date")
		return
	}

	closure, err := s.queries.UpdateClosure(r.Context(), id, start, end, req.Reason)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "closure not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update closure")
		return
	}
	writeJSON(w, http.StatusOK, closureToMap(closure))
}

func (s *Server) handleAdminDeleteClosure(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid closure id")
		return
	}
	if err := s.queries.DeleteClosure(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete closure")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseID(r *http.Request) (int32, error) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 32)
	return int32(id), err
}

func settingsToMap(s *db.Settings) map[string]any {
	return map[string]any{
		"resource_name":         s.ResourceName,
		"hourly_rate_cents":     s.HourlyRateCents,
		"currency":              s.Currency,
		"timezone":              s.Timezone,
		"bookable_start":        s.BookableStart.Format("15:04"),
		"bookable_end":          s.BookableEnd.Format("15:04"),
		"min_hours":             s.MinHours,
		"max_hours":             s.MaxHours,
		"reminder_hours_before": s.ReminderHoursBefore,
		"notification_emails":   s.NotificationEmails,
	}
}

func closureToMap(c *db.Closure) map[string]any {
	return map[string]any{
		"id":         c.ID,
		"start_date": c.StartDate.Format("2006-01-02"),
		"end_date":   c.EndDate.Format("2006-01-02"),
		"reason":     c.Reason,
		"created_at": c.CreatedAt,
	}
}
