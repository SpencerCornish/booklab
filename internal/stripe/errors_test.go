package stripe

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stripe/stripe-go/v85"
)

func TestIsDefiniteFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"card declined", &stripe.Error{Type: stripe.ErrorTypeCard, HTTPStatusCode: http.StatusPaymentRequired}, true},
		{"wrapped card declined", fmt.Errorf("create payment intent: %w", &stripe.Error{Type: stripe.ErrorTypeCard}), true},
		{"invalid request", &stripe.Error{Type: stripe.ErrorTypeInvalidRequest, HTTPStatusCode: http.StatusBadRequest}, true},
		{"lock conflict", &stripe.Error{Type: stripe.ErrorTypeInvalidRequest, HTTPStatusCode: http.StatusConflict}, false},
		{"rate limited", &stripe.Error{Type: stripe.ErrorTypeInvalidRequest, HTTPStatusCode: http.StatusTooManyRequests}, false},
		{"idempotency mismatch", &stripe.Error{Type: stripe.ErrorTypeIdempotency, HTTPStatusCode: http.StatusBadRequest}, false},
		{"stripe server error", &stripe.Error{Type: stripe.ErrorTypeAPI, HTTPStatusCode: http.StatusInternalServerError}, false},
		{"network error", errors.New("dial tcp: i/o timeout"), false},
	}
	for _, tc := range cases {
		if got := IsDefiniteFailure(tc.err); got != tc.want {
			t.Errorf("%s: IsDefiniteFailure = %v, want %v", tc.name, got, tc.want)
		}
	}
}
