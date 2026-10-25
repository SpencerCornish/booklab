// Package stripetest provides the single Stripe.Client test double shared by
// every suite: the deterministic fake from internal/stripe (prefixed,
// counter-suffixed IDs, rejection of unknown setup intents) plus a counting
// wrapper for tests that assert exactly-once charging.
package stripetest

import (
	"sync/atomic"

	"github.com/spencercornish/booklab/internal/stripe"
)

// New returns the deterministic fake client.
func New() stripe.Client {
	return stripe.NewFake()
}

// Counting wraps the fake and counts ChargePaymentMethod invocations.
type Counting struct {
	stripe.Client
	charges atomic.Int64
}

// NewCounting returns a fake that records how many times it charged.
func NewCounting() *Counting {
	return &Counting{Client: stripe.NewFake()}
}

func (c *Counting) ChargePaymentMethod(pmID string, amountCents int64, currency, description, idempotencyKey string) (piID, receiptURL string, err error) {
	c.charges.Add(1)
	return c.Client.ChargePaymentMethod(pmID, amountCents, currency, description, idempotencyKey)
}

// ChargeCalls reports how many charge attempts were made.
func (c *Counting) ChargeCalls() int64 {
	return c.charges.Load()
}
