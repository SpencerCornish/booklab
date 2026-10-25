package stripe

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// fake is a hermetic Client implementation for functional tests. It never
// touches the network: every method returns deterministic, counter-suffixed
// IDs so multiple calls never collide (e.g. booking idempotency by
// setup_intent_id).
type fake struct {
	counter atomic.Int64
}

// NewFake returns a Client that simulates Stripe responses locally.
// Enable via STRIPE_FAKE=1; no real API calls are made.
func NewFake() Client {
	return &fake{}
}

func (f *fake) next(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, f.counter.Add(1))
}

func (f *fake) CreateSetupIntent(email string) (setupIntentID, clientSecret string, err error) {
	id := f.next("seti_fake")
	return id, "cs_fake_" + id, nil
}

// GetPaymentMethodFromSetupIntent accepts only IDs this fake issued (prefix
// seti_fake_), matching real Stripe's rejection of unknown intents.
func (f *fake) GetPaymentMethodFromSetupIntent(setupIntentID string) (string, error) {
	if !strings.HasPrefix(setupIntentID, "seti_fake_") {
		return "", fmt.Errorf("setup intent not succeeded")
	}
	return "pm_fake_" + setupIntentID, nil
}

func (f *fake) ChargePaymentMethod(pmID string, amountCents int64, currency, description, idempotencyKey string) (piID, receiptURL string, err error) {
	if pmID == "" {
		return "", "", fmt.Errorf("get payment method: no payment method")
	}
	pi := f.next("pi_fake")
	return pi, "https://dashboard.stripe.com/test/receipts/" + pi, nil
}

func (f *fake) DetachPaymentMethod(pmID string) error {
	return nil
}
