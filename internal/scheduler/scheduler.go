package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/spencercornish/booklab/internal/db"
	"github.com/spencercornish/booklab/internal/email"
	"github.com/spencercornish/booklab/internal/stripe"
)

type Scheduler struct {
	queries *db.Queries
	email   *email.Service
	stripe  stripe.Client
	appURL  string
	logger  *slog.Logger
}

func New(queries *db.Queries, emailSvc *email.Service, stripeSvc stripe.Client, appURL string, log *slog.Logger) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{
		queries: queries, email: emailSvc, stripe: stripeSvc, appURL: appURL,
		logger: log.With("component", "scheduler"),
	}
}

// Start runs a background ticker that checks for reminder emails and auto-charges.
// It runs every 30 minutes. Call this in a goroutine.
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	// Run immediately on start, then on tick.
	s.runOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *Scheduler) runOnce(ctx context.Context) {
	s.logger.Info("scheduler_tick_started")
	settings, err := s.queries.GetSettings(ctx)
	if err != nil {
		s.logger.Error("scheduler get settings failed", "error", err)
		return
	}

	completed := s.completeExpiredBookings(ctx, settings)
	remindersSent := s.sendReminders(ctx, settings)
	chargesOK := s.autoChargeCompleted(ctx, settings)
	s.logger.Info("scheduler_tick_completed", "bookings_completed", completed, "reminders_sent", remindersSent, "charges_succeeded", chargesOK)
}

func (s *Scheduler) completeExpiredBookings(ctx context.Context, settings *db.Settings) int {
	bookings, err := s.queries.CompleteExpiredBookings(ctx)
	if err != nil {
		s.logger.Error("scheduler complete expired bookings failed", "error", err)
		return 0
	}
	for _, b := range bookings {
		b := b
		go func() {
			hours := int32(b.EndTime.Sub(b.StartTime).Hours())
			if hours < 1 {
				hours = 1
			}
			delay := time.Duration(settings.AutoChargeDelayMinutes) * time.Minute
			autoChargeAt := b.EndTime.Add(delay)
			if b.CompletedAt != nil {
				autoChargeAt = b.CompletedAt.Add(delay)
			}
			staffData := email.StaffCompletionData{
				ResourceName:    settings.ResourceName,
				BookerName:      b.Name,
				BookerEmail:     b.Email,
				StartTime:       b.StartTime,
				EndTime:         b.EndTime,
				AutoAmountCents: hours * settings.HourlyRateCents,
				AutoChargeAt:    autoChargeAt,
				AdminURL:        s.appURL + "/admin/bookings",
				Timezone:        settings.Timezone,
			}
			for _, addr := range splitEmails(settings.NotificationEmails) {
				if err := s.email.SendStaffCompletion(addr, staffData); err != nil {
					s.logger.Error("scheduler send staff completion email failed", "booking_id", b.ID, "recipient", addr, "error", err)
				}
			}
		}()
	}
	return len(bookings)
}

func (s *Scheduler) sendReminders(ctx context.Context, settings *db.Settings) int {
	bookings, err := s.queries.ListBookingsDueReminder(ctx, int(settings.ReminderHoursBefore))
	if err != nil {
		s.logger.Error("scheduler list bookings due reminder failed", "error", err)
		return 0
	}

	sent := 0
	for _, b := range bookings {
		data := email.ReminderData{
			ResourceName: settings.ResourceName,
			BookerName:   b.Name,
			StartTime:    b.StartTime,
			EndTime:      b.EndTime,
			CancelURL:    fmt.Sprintf("%s/cancel/%s", s.appURL, b.CancelToken),
			Timezone:     settings.Timezone,
		}
		if err := s.email.SendReminder(b.Email, data); err != nil {
			s.logger.Error("scheduler send reminder failed", "booking_id", b.ID, "email", b.Email, "error", err)
			continue
		}
		if err := s.queries.MarkReminderSent(ctx, b.ID); err != nil {
			s.logger.Error("scheduler mark reminder sent failed", "booking_id", b.ID, "email", b.Email, "error", err)
			continue
		}
		sent++
	}
	return sent
}

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

// $200 is a safe max for now
const maxChargeAmountCents int32 = 20_000

func (s *Scheduler) autoChargeCompleted(ctx context.Context, settings *db.Settings) int {
	charged := 0
	for {
		b, err := s.queries.ClaimBookingForAutoCharge(ctx, settings.AutoChargeDelayMinutes)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return charged
			}
			s.logger.Error("scheduler claim booking for auto-charge failed", "error", err)
			return charged
		}

		hours := int32(b.EndTime.Sub(b.StartTime).Hours())
		if hours < 1 {
			hours = 1
		}
		amountCents := hours * settings.HourlyRateCents
		if amountCents <= 0 || amountCents > maxChargeAmountCents {
			msg := fmt.Sprintf("computed charge amount %d cents is out of allowed range", amountCents)
			_ = s.queries.RevertBookingFromChargingToCompleted(ctx, b.ID, msg)
			s.logger.Warn("scheduler auto-charge amount out of range",
				"booking_id", b.ID, "email", b.Email, "amount_cents", amountCents)
			continue
		}

		description := settings.ResourceName + " – " + b.StartTime.Format("Jan 2, 2006") + " (auto)"
		idempotencyKey := fmt.Sprintf("charge-booking-%d-auto", b.ID)

		piID, receiptURL, err := s.stripe.ChargePaymentMethod(*b.StripePaymentMethodID, int64(amountCents), settings.Currency, description, idempotencyKey)
		if err != nil {
			_ = s.queries.RevertBookingFromChargingToCompleted(ctx, b.ID, err.Error())
			s.logger.Error("scheduler auto-charge stripe failed", "booking_id", b.ID, "email", b.Email, "error", err)
			go func(booking *db.Booking, amount int32, chargeErr string) {
				failureData := email.StaffChargeFailureData{
					ResourceName:   settings.ResourceName,
					BookerName:     booking.Name,
					BookerEmail:    booking.Email,
					StartTime:      booking.StartTime,
					EndTime:        booking.EndTime,
					AmountCents:    amount,
					ChargeAttempts: booking.ChargeAttempts + 1,
					ErrorMessage:   chargeErr,
					Source:         "auto",
					AdminURL:       s.appURL + "/admin/bookings",
					Timezone:       settings.Timezone,
				}
				for _, addr := range splitEmails(settings.NotificationEmails) {
					if emailErr := s.email.SendStaffChargeFailure(addr, failureData); emailErr != nil {
						s.logger.Error("scheduler send charge failure email failed", "booking_id", booking.ID, "recipient", addr, "error", emailErr)
					}
				}
			}(b, amountCents, err.Error())
			continue
		}

		updated, err := s.queries.UpdateBookingCharged(ctx, b.ID, piID, receiptURL, amountCents)
		if err != nil {
			// Stripe already charged — do not revert to 'completed' (would cause a double charge on retry).
			// Leave the row in 'charging' so a human can reconcile, and log enough to find it.
			if errors.Is(err, pgx.ErrNoRows) {
				s.logger.Error("scheduler auto-charge db finalize failed after stripe success — booking stuck in charging, requires manual reconciliation",
					"booking_id", b.ID, "email", b.Email, "payment_intent_id", piID)
			} else {
				s.logger.Error("scheduler record auto-charge failed after stripe success — booking stuck in charging, requires manual reconciliation",
					"booking_id", b.ID, "email", b.Email, "payment_intent_id", piID, "error", err)
			}
			continue
		}

		go func(booking *db.Booking, amount int32, receipt string) {
			data := email.ReceiptData{
				ResourceName:     settings.ResourceName,
				BookerName:       booking.Name,
				StartTime:        booking.StartTime,
				EndTime:          booking.EndTime,
				AmountCents:      amount,
				StripeReceiptURL: receipt,
				Timezone:         settings.Timezone,
			}
			if err := s.email.SendReceipt(booking.Email, data); err != nil {
				s.logger.Error("scheduler send receipt failed", "booking_id", booking.ID, "email", booking.Email, "error", err)
			}
		}(updated, amountCents, receiptURL)

		s.logger.Info("scheduler auto-charged booking",
			"booking_id", b.ID, "email", b.Email, "amount_cents", amountCents, "payment_intent_id", piID)
		charged++
	}
}
