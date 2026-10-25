package stripetest

import (
	"sync"

	"github.com/spencercornish/booklab/internal/stripe"
)

// Scripted wraps the deterministic fake and returns queued errors from
// successive charges while recording each idempotency key, for tests that
// assert retry behavior (e.g. that an unknown-outcome retry reuses the key).
type Scripted struct {
	stripe.Client

	mu   sync.Mutex
	errs []error
	keys []string
}

// NewScripted returns a fake whose successive charges return errs in order;
// charges past the end of errs succeed.
func NewScripted(errs ...error) *Scripted {
	return &Scripted{Client: stripe.NewFake(), errs: errs}
}

func (s *Scripted) ChargePaymentMethod(pmID string, amountCents int64, currency, description, idempotencyKey string) (piID, receiptURL string, err error) {
	s.mu.Lock()
	s.keys = append(s.keys, idempotencyKey)
	if len(s.errs) > 0 {
		err = s.errs[0]
		s.errs = s.errs[1:]
	}
	s.mu.Unlock()
	if err != nil {
		return "", "", err
	}
	return s.Client.ChargePaymentMethod(pmID, amountCents, currency, description, idempotencyKey)
}

// Keys reports the idempotency keys of every charge attempted so far.
func (s *Scripted) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.keys...)
}
