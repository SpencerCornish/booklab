package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/spencercornish/booklab/internal/db"
	"github.com/spencercornish/booklab/internal/email"
)

type Scheduler struct {
	queries *db.Queries
	email   *email.Service
	appURL  string
}

func New(queries *db.Queries, emailSvc *email.Service, appURL string) *Scheduler {
	return &Scheduler{queries: queries, email: emailSvc, appURL: appURL}
}

// Start runs a background ticker that checks for bookings needing reminder emails.
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
