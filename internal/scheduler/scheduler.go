package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/spencercornish/booklab/internal/db"
	"github.com/spencercornish/booklab/internal/email"
	"github.com/spencercornish/booklab/internal/stripe"
)

type Scheduler struct {
	queries *db.Queries
	email   *email.Service
	stripe  *stripe.Service
	appURL  string
}

func New(queries *db.Queries, emailSvc *email.Service, stripeSvc *stripe.Service, appURL string) *Scheduler {
	return &Scheduler{queries: queries, email: emailSvc, stripe: stripeSvc, appURL: appURL}
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
	settings, err := s.queries.GetSettings(ctx)
	if err != nil {
		log.Printf("scheduler: get settings: %v", err)
		return
	}

	s.sendReminders(ctx, settings)
	s.autoChargeCompleted(ctx, settings)
}

func (s *Scheduler) sendReminders(ctx context.Context, settings *db.Settings) {
	bookings, err := s.queries.ListBookingsDueReminder(ctx, int(settings.ReminderHoursBefore))
	if err != nil {
		log.Printf("scheduler: list bookings due reminder: %v", err)
		return
	}

	for _, b := range bookings {
		data := email.ReminderData{
			ResourceName: settings.ResourceName,
			BookerName:   b.Name,
			StartTime:    b.StartTime,
			EndTime:      b.EndTime,
			CancelURL:    fmt.Sprintf("%s/cancel/%s", s.appURL, b.CancelToken),
		}
		if err := s.email.SendReminder(b.Email, data); err != nil {
			log.Printf("scheduler: send reminder to %s: %v", b.Email, err)
			continue
		}
		if err := s.queries.MarkReminderSent(ctx, b.ID); err != nil {
			log.Printf("scheduler: mark reminder sent for booking %d: %v", b.ID, err)
		}
	}
}

func (s *Scheduler) autoChargeCompleted(ctx context.Context, settings *db.Settings) {
	bookings, err := s.queries.ListBookingsDueAutoCharge(ctx)
	if err != nil {
		log.Printf("scheduler: list bookings due auto-charge: %v", err)
		return
	}

	for _, b := range bookings {
		hours := int32(b.EndTime.Sub(b.StartTime).Hours())
		if hours < 1 {
			hours = 1
		}
		amountCents := hours * settings.HourlyRateCents
		description := settings.ResourceName + " – " + b.StartTime.Format("Jan 2, 2006") + " (auto)"

		piID, receiptURL, err := s.stripe.ChargePaymentMethod(*b.StripePaymentMethodID, int64(amountCents), settings.Currency, description)
		if err != nil {
			log.Printf("scheduler: auto-charge booking %d: %v", b.ID, err)
			continue
		}

		updated, err := s.queries.UpdateBookingCharged(ctx, b.ID, piID, receiptURL, amountCents)
		if err != nil {
			log.Printf("scheduler: record auto-charge for booking %d: %v", b.ID, err)
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
			}
			if err := s.email.SendReceipt(booking.Email, data); err != nil {
				log.Printf("scheduler: send receipt to %s: %v", booking.Email, err)
			}
		}(updated, amountCents, receiptURL)

		log.Printf("scheduler: auto-charged booking %d (%s) for %s", b.ID, b.Email, description)
	}
}
