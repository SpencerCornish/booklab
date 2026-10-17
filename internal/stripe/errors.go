package stripe

import (
	"errors"
	"net/http"

	"github.com/stripe/stripe-go/v85"
)

// IsDefiniteFailure reports whether an error from ChargePaymentMethod is a definite answer
// from Stripe that no payment was taken: a card decline or a rejected request. Network
// errors, Stripe server errors, rate limits and idempotency conflicts leave the outcome
// unknown, because the charge may have gone through; a retry must then reuse the same
// idempotency key so Stripe replays the original result instead of charging again.
func IsDefiniteFailure(err error) bool {
	var se *stripe.Error
	if !errors.As(err, &se) {
		return false
	}
	switch se.Type {
	case stripe.ErrorTypeCard:
		return true
	case stripe.ErrorTypeInvalidRequest:
		return se.HTTPStatusCode != http.StatusConflict && se.HTTPStatusCode != http.StatusTooManyRequests
	default:
		return false
	}
}
