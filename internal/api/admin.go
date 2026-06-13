package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/spencercornish/booklab/internal/db"
	emailsvc "github.com/spencercornish/booklab/internal/email"
)

const (
	loginRateLimitWindow   = 15 * time.Minute
	loginRateLimitMax      = 10
	minAdminPasswordLength = 8
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
	if !s.readJSONRequest(w, r, &req) {
		return
	}

	ctx := r.Context()
	since := time.Now().Add(-loginRateLimitWindow)
	ip := clientIP(r)

	ipFails, err := s.queries.CountRecentFailedLoginAttemptsByIP(ctx, ip, since)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "login failed", err)
		return
	}
	userFails, err := s.queries.CountRecentFailedLoginAttemptsByUsername(ctx, req.Username, since)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "login failed", err)
		return
	}
	if ipFails >= loginRateLimitMax || userFails >= loginRateLimitMax {
		s.writeError(w, r, http.StatusTooManyRequests, "too many failed login attempts, try again later", nil)
		return
	}

	user, err := s.queries.GetAdminByUsername(ctx, req.Username)
	if err != nil {
		if err == pgx.ErrNoRows {
			_ = s.queries.RecordLoginAttempt(ctx, req.Username, ip, false)
			s.writeError(w, r, http.StatusUnauthorized, "invalid credentials", nil)
			return
		}
		s.writeError(w, r, http.StatusInternalServerError, "login failed", err)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		_ = s.queries.RecordLoginAttempt(ctx, req.Username, ip, false)
		s.writeError(w, r, http.StatusUnauthorized, "invalid credentials", nil)
		return
	}

	if err := s.queries.RecordLoginAttempt(ctx, req.Username, ip, true); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "login failed", err)
		return
	}

	token, err := s.newAdminSession(ctx, user.Username)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "login failed", err)
		return
	}
	setSessionCookie(w, token)
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

func adminUserToMap(u *db.AdminUserPublic) map[string]any {
	return map[string]any{
		"id":         u.ID,
		"username":   u.Username,
		"created_at": u.CreatedAt,
	}
}

func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	currentUsername, ok := adminUsernameFromContext(r.Context())
	if !ok {
		s.writeError(w, r, http.StatusUnauthorized, "authentication required", nil)
		return
	}

	users, err := s.queries.ListAdminUsers(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to list users", err)
		return
	}

	result := make([]map[string]any, len(users))
	for i, u := range users {
		result[i] = adminUserToMap(u)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users":            result,
		"current_username": currentUsername,
	})
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !s.readJSONRequest(w, r, &req) {
		return
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		s.writeError(w, r, http.StatusBadRequest, "username is required", nil)
		return
	}
	if len(req.Password) < minAdminPasswordLength {
		s.writeError(w, r, http.StatusBadRequest, fmt.Sprintf("password must be at least %d characters", minAdminPasswordLength), nil)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to create user", err)
		return
	}

	user, err := s.queries.CreateAdminUser(r.Context(), username, string(hash))
	if err != nil {
		if isUniqueViolation(err) {
			s.writeError(w, r, http.StatusConflict, "username already exists", err)
			return
		}
		s.writeError(w, r, http.StatusInternalServerError, "failed to create user", err)
		return
	}

	writeJSON(w, http.StatusCreated, adminUserToMap(&db.AdminUserPublic{
		ID:        user.ID,
		Username:  user.Username,
		CreatedAt: user.CreatedAt,
	}))
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	currentUsername, ok := adminUsernameFromContext(r.Context())
	if !ok {
		s.writeError(w, r, http.StatusUnauthorized, "authentication required", nil)
		return
	}

	target := chi.URLParam(r, "username")
	if target == "" {
		s.writeError(w, r, http.StatusBadRequest, "username is required", nil)
		return
	}
	if target == currentUsername {
		s.writeError(w, r, http.StatusBadRequest, "cannot delete your own account", nil)
		return
	}

	if err := s.queries.DeleteAdminUser(r.Context(), target); err != nil {
		switch {
		case errors.Is(err, db.ErrLastAdminUser):
			s.writeError(w, r, http.StatusBadRequest, "cannot delete the last admin user", nil)
		case errors.Is(err, pgx.ErrNoRows):
			s.writeError(w, r, http.StatusNotFound, "user not found", err)
		default:
			s.writeError(w, r, http.StatusInternalServerError, "failed to delete user", err)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminChangePassword(w http.ResponseWriter, r *http.Request) {
	currentUsername, ok := adminUsernameFromContext(r.Context())
	if !ok {
		s.writeError(w, r, http.StatusUnauthorized, "authentication required", nil)
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !s.readJSONRequest(w, r, &req) {
		return
	}

	if req.CurrentPassword == "" {
		s.writeError(w, r, http.StatusBadRequest, "current_password is required", nil)
		return
	}
	if len(req.NewPassword) < minAdminPasswordLength {
		s.writeError(w, r, http.StatusBadRequest, fmt.Sprintf("new_password must be at least %d characters", minAdminPasswordLength), nil)
		return
	}

	user, err := s.queries.GetAdminByUsername(r.Context(), currentUsername)
	if err != nil {
		if err == pgx.ErrNoRows {
			s.writeError(w, r, http.StatusUnauthorized, "authentication required", err)
			return
		}
		s.writeError(w, r, http.StatusInternalServerError, "failed to change password", err)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		s.writeError(w, r, http.StatusUnauthorized, "current password is incorrect", nil)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to change password", err)
		return
	}

	if err := s.queries.UpdateAdminUserPassword(r.Context(), currentUsername, string(hash)); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to change password", err)
		return
	}

	// Revoke every existing session for this user so a stolen/old cookie cannot
	// outlive the password change, then issue a fresh session for the caller so
	// they remain logged in.
	if err := s.queries.DeleteAdminSessionsByUsername(r.Context(), currentUsername); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to change password", err)
		return
	}
	token, err := s.newAdminSession(r.Context(), currentUsername)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to change password", err)
		return
	}
	setSessionCookie(w, token)

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminListBookings(w http.ResponseWriter, r *http.Request) {
	params := db.ListBookingsParams{}

	if d := r.URL.Query().Get("date"); d != "" {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			s.writeError(w, r, http.StatusBadRequest, "invalid date format", err)
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
		s.writeError(w, r, http.StatusInternalServerError, "failed to list bookings", err)
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
		s.writeError(w, r, http.StatusBadRequest, "invalid booking id", err)
		return
	}

	var req struct {
		EndTime *time.Time        `json:"end_time"`
		Status  *db.BookingStatus `json:"status"`
	}
	if !s.readJSONRequest(w, r, &req) {
		return
	}

	var booking *db.Booking
	if req.EndTime != nil {
		booking, err = s.queries.UpdateBookingEndTime(r.Context(), id, *req.EndTime)
	} else if req.Status != nil {
		booking, err = s.queries.UpdateBookingStatus(r.Context(), id, *req.Status)
	} else {
		s.writeError(w, r, http.StatusBadRequest, "no fields to update", nil)
		return
	}

	if err != nil {
		if err == pgx.ErrNoRows {
			s.writeError(w, r, http.StatusNotFound, "booking not found", err)
			return
		}
		s.writeError(w, r, http.StatusInternalServerError, "failed to update booking", err)
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
				AutoChargeAt:    time.Now().Add(time.Duration(settings.AutoChargeDelayMinutes) * time.Minute),
				AdminURL:        s.cfg.AppURL + "/admin/bookings",
				Timezone:        settings.Timezone,
			}
			for _, addr := range splitEmails(settings.NotificationEmails) {
				if err := s.email.SendStaffCompletion(addr, staffData); err != nil {
					s.logger.Error("staff completion email failed", "booking_id", booking.ID, "recipient", addr, "error", err)
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
		s.writeError(w, r, http.StatusBadRequest, "invalid booking id", err)
		return
	}

	var req struct {
		AmountCents *int32 `json:"amount_cents"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		s.writeError(w, r, http.StatusBadRequest, "invalid request body", err)
		return
	}

	booking, err := s.queries.ClaimBookingForCharge(r.Context(), id)
	if err != nil {
		if err == pgx.ErrNoRows {
			if _, gerr := s.queries.GetBookingByID(r.Context(), id); gerr == pgx.ErrNoRows {
				s.writeError(w, r, http.StatusNotFound, "booking not found", gerr)
				return
			}
			s.writeError(w, r, http.StatusConflict, "booking cannot be charged (already charged, in progress, or not eligible)", nil)
			return
		}
		s.writeError(w, r, http.StatusInternalServerError, "failed to claim booking for charge", err)
		return
	}

	settings, err := s.queries.GetSettings(r.Context())
	if err != nil {
		_ = s.queries.RevertBookingFromChargingToCompleted(r.Context(), id, "internal: failed to load settings")
		s.writeError(w, r, http.StatusInternalServerError, "failed to load settings", err)
		return
	}

	var amountCents int32
	if req.AmountCents != nil {
		if *req.AmountCents <= 0 || *req.AmountCents > maxChargeAmountCents {
			_ = s.queries.RevertBookingFromChargingToCompleted(r.Context(), id, "internal: invalid amount_cents provided")
			s.writeError(w, r, http.StatusBadRequest, "invalid amount_cents", nil)
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
			_ = s.queries.RevertBookingFromChargingToCompleted(r.Context(), id, "internal: computed charge amount is out of allowed range")
			s.writeError(w, r, http.StatusBadRequest, "computed charge amount is out of allowed range", nil)
			return
		}
	}

	description := settings.ResourceName + " – " + booking.StartTime.Format("Jan 2, 2006")
	idempotencyKey := fmt.Sprintf("charge-booking-%d-admin", id)
	piID, receiptURL, err := s.stripe.ChargePaymentMethod(*booking.StripePaymentMethodID, int64(amountCents), settings.Currency, description, idempotencyKey)
	if err != nil {
		_ = s.queries.RevertBookingFromChargingToCompleted(r.Context(), id, err.Error())
		s.logger.Error("admin charge stripe failed", "booking_id", id, "email", booking.Email, "error", err)
		go func(chargeErr string) {
			failureData := emailsvc.StaffChargeFailureData{
				ResourceName:   settings.ResourceName,
				BookerName:     booking.Name,
				BookerEmail:    booking.Email,
				StartTime:      booking.StartTime,
				EndTime:        booking.EndTime,
				AmountCents:    amountCents,
				ChargeAttempts: booking.ChargeAttempts + 1,
				ErrorMessage:   chargeErr,
				Source:         "manual",
				AdminURL:       s.cfg.AppURL + "/admin/bookings",
				Timezone:       settings.Timezone,
			}
			for _, addr := range splitEmails(settings.NotificationEmails) {
				if emailErr := s.email.SendStaffChargeFailure(addr, failureData); emailErr != nil {
					s.logger.Error("admin send charge failure email failed", "booking_id", id, "recipient", addr, "error", emailErr)
				}
			}
		}(err.Error())
		s.writeError(w, r, http.StatusInternalServerError, "payment failed", err)
		return
	}

	updated, err := s.queries.UpdateBookingCharged(r.Context(), id, piID, receiptURL, amountCents)
	if err != nil {
		if err == pgx.ErrNoRows {
			s.logger.Error("admin charge db finalize missing row after stripe success",
				"booking_id", id, "payment_intent_id", piID)
		}
		s.writeError(w, r, http.StatusInternalServerError, "failed to record charge", err)
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
			Timezone:         settings.Timezone,
		}
		if err := s.email.SendReceipt(booking.Email, data); err != nil {
			s.logger.Error("admin send receipt failed", "booking_id", id, "email", booking.Email, "error", err)
		}
	}()

	writeJSON(w, http.StatusOK, bookingToAdmin(updated))
}

func (s *Server) handleAdminGetInsights(w http.ResponseWriter, r *http.Request) {
	insights, err := s.queries.GetBookingInsights(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load insights", err)
		return
	}
	writeJSON(w, http.StatusOK, insights)
}

func (s *Server) handleAdminGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.queries.GetSettings(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load settings", err)
		return
	}
	writeJSON(w, http.StatusOK, settingsToMap(settings))
}

func (s *Server) handleAdminUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceName             *string `json:"resource_name"`
		HourlyRateCents          *int32  `json:"hourly_rate_cents"`
		Currency                 *string `json:"currency"`
		Timezone                 *string `json:"timezone"`
		BookableStart            *string `json:"bookable_start"`
		BookableEnd              *string `json:"bookable_end"`
		MinHours                 *int32  `json:"min_hours"`
		MaxHours                 *int32  `json:"max_hours"`
		ReminderHoursBefore      *int32  `json:"reminder_hours_before"`
		NotificationEmails       *string   `json:"notification_emails"`
		AutoChargeDelayMinutes   *int32    `json:"auto_charge_delay_minutes"`
		ReferralSources          *[]string `json:"referral_sources"`
		TermsContent             *string   `json:"terms_content"`
		PrivacyContent           *string   `json:"privacy_content"`
	}
	if !s.readJSONRequest(w, r, &req) {
		return
	}

	params := db.UpdateSettingsParams{
		ResourceName:             req.ResourceName,
		HourlyRateCents:          req.HourlyRateCents,
		Currency:                 req.Currency,
		Timezone:                 req.Timezone,
		MinHours:                 req.MinHours,
		MaxHours:                 req.MaxHours,
		ReminderHoursBefore:      req.ReminderHoursBefore,
		NotificationEmails:       req.NotificationEmails,
		AutoChargeDelayMinutes:   req.AutoChargeDelayMinutes,
		ReferralSources:          req.ReferralSources,
		TermsContent:             req.TermsContent,
		PrivacyContent:           req.PrivacyContent,
	}

	if req.BookableStart != nil {
		t, err := time.Parse("15:04", *req.BookableStart)
		if err != nil {
			s.writeError(w, r, http.StatusBadRequest, "invalid bookable_start format (HH:MM)", err)
			return
		}
		params.BookableStart = &t
	}
	if req.BookableEnd != nil {
		t, err := time.Parse("15:04", *req.BookableEnd)
		if err != nil {
			s.writeError(w, r, http.StatusBadRequest, "invalid bookable_end format (HH:MM)", err)
			return
		}
		params.BookableEnd = &t
	}

	settings, err := s.queries.UpdateSettings(r.Context(), params)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to update settings", err)
		return
	}
	writeJSON(w, http.StatusOK, settingsToMap(settings))
}

func (s *Server) handleAdminListClosures(w http.ResponseWriter, r *http.Request) {
	closures, err := s.queries.ListClosures(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to list closures", err)
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
		AllDay    *bool   `json:"all_day"`
		StartTime *string `json:"start_time"`
		EndTime   *string `json:"end_time"`
		Reason    *string `json:"reason"`
	}
	if !s.readJSONRequest(w, r, &req) {
		return
	}

	start, end, allDay, startTime, endTime, badReq, err := parseClosureUpsert(req.StartDate, req.EndDate, req.AllDay, req.StartTime, req.EndTime)
	if badReq != "" {
		s.writeError(w, r, http.StatusBadRequest, badReq, err)
		return
	}
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid closure dates", err)
		return
	}

	closure, err := s.queries.CreateClosure(r.Context(), start, end, allDay, startTime, endTime, req.Reason)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to create closure", err)
		return
	}
	writeJSON(w, http.StatusCreated, closureToMap(closure))
}

func (s *Server) handleAdminUpdateClosure(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid closure id", err)
		return
	}

	var req struct {
		StartDate string  `json:"start_date"`
		EndDate   string  `json:"end_date"`
		AllDay    *bool   `json:"all_day"`
		StartTime *string `json:"start_time"`
		EndTime   *string `json:"end_time"`
		Reason    *string `json:"reason"`
	}
	if !s.readJSONRequest(w, r, &req) {
		return
	}

	start, end, allDay, startTime, endTime, badReq, err := parseClosureUpsert(req.StartDate, req.EndDate, req.AllDay, req.StartTime, req.EndTime)
	if badReq != "" {
		s.writeError(w, r, http.StatusBadRequest, badReq, err)
		return
	}
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid closure dates", err)
		return
	}

	closure, err := s.queries.UpdateClosure(r.Context(), id, start, end, allDay, startTime, endTime, req.Reason)
	if err != nil {
		if err == pgx.ErrNoRows {
			s.writeError(w, r, http.StatusNotFound, "closure not found", err)
			return
		}
		s.writeError(w, r, http.StatusInternalServerError, "failed to update closure", err)
		return
	}
	writeJSON(w, http.StatusOK, closureToMap(closure))
}

func (s *Server) handleAdminDeleteClosure(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid closure id", err)
		return
	}
	if err := s.queries.DeleteClosure(r.Context(), id); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to delete closure", err)
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
		"resource_name":              s.ResourceName,
		"hourly_rate_cents":          s.HourlyRateCents,
		"currency":                   s.Currency,
		"timezone":                   s.Timezone,
		"bookable_start":             s.BookableStart.Format("15:04"),
		"bookable_end":               s.BookableEnd.Format("15:04"),
		"min_hours":                  s.MinHours,
		"max_hours":                  s.MaxHours,
		"reminder_hours_before":      s.ReminderHoursBefore,
		"notification_emails":        s.NotificationEmails,
		"auto_charge_delay_minutes":  s.AutoChargeDelayMinutes,
		"referral_sources":           s.ReferralSources,
		"terms_content":              s.TermsContent,
		"privacy_content":            s.PrivacyContent,
	}
}

func closureToMap(c *db.Closure) map[string]any {
	m := map[string]any{
		"id":         c.ID,
		"start_date": c.StartDate.Format("2006-01-02"),
		"end_date":   c.EndDate.Format("2006-01-02"),
		"all_day":    c.AllDay,
		"reason":     c.Reason,
		"created_at": c.CreatedAt,
	}
	if c.StartTime != nil {
		m["start_time"] = c.StartTime.Format("15:04")
	} else {
		m["start_time"] = nil
	}
	if c.EndTime != nil {
		m["end_time"] = c.EndTime.Format("15:04")
	} else {
		m["end_time"] = nil
	}
	return m
}

func parseClosureUpsert(startDate, endDate string, allDay *bool, startTimeStr, endTimeStr *string) (start, end time.Time, allDayOut bool, startTime, endTime *time.Time, badRequest string, err error) {
	allDayOut = true
	if allDay != nil {
		allDayOut = *allDay
	}

	start, err = time.Parse("2006-01-02", startDate)
	if err != nil {
		return time.Time{}, time.Time{}, false, nil, nil, "invalid start_date (YYYY-MM-DD)", err
	}
	end, err = time.Parse("2006-01-02", endDate)
	if err != nil {
		return time.Time{}, time.Time{}, false, nil, nil, "invalid end_date (YYYY-MM-DD)", err
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, false, nil, nil, "end_date must be on or after start_date", nil
	}

	if allDayOut {
		return start, end, true, nil, nil, "", nil
	}

	if startTimeStr == nil || *startTimeStr == "" || endTimeStr == nil || *endTimeStr == "" {
		return time.Time{}, time.Time{}, false, nil, nil, "start_time and end_time are required when all_day is false", nil
	}

	st, err := time.Parse("15:04", *startTimeStr)
	if err != nil {
		return time.Time{}, time.Time{}, false, nil, nil, "invalid start_time (HH:MM)", err
	}
	et, err := time.Parse("15:04", *endTimeStr)
	if err != nil {
		return time.Time{}, time.Time{}, false, nil, nil, "invalid end_time (HH:MM)", err
	}
	if !et.After(st) {
		return time.Time{}, time.Time{}, false, nil, nil, "end_time must be after start_time", nil
	}

	return start, end, false, &st, &et, "", nil
}
