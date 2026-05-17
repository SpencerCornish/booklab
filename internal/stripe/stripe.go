package stripe

import (
	"fmt"

	"github.com/stripe/stripe-go/v85"
	"github.com/stripe/stripe-go/v85/customer"
	"github.com/stripe/stripe-go/v85/paymentintent"
	"github.com/stripe/stripe-go/v85/paymentmethod"
	"github.com/stripe/stripe-go/v85/setupintent"
)

type Service struct{}

func New(secretKey string) *Service {
	stripe.Key = secretKey
	return &Service{}
}

// CreateSetupIntent creates a SetupIntent for saving a card without charging.
func (s *Service) CreateSetupIntent(email string) (string, string, error) {
	// Create or find a customer
	cParams := &stripe.CustomerParams{
		Email: stripe.String(email),
	}
	c, err := customer.New(cParams)
	if err != nil {
		return "", "", fmt.Errorf("create customer: %w", err)
	}

	siParams := &stripe.SetupIntentParams{
		Customer:           stripe.String(c.ID),
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
	}
	si, err := setupintent.New(siParams)
	if err != nil {
		return "", "", fmt.Errorf("create setup intent: %w", err)
	}
	return si.ID, si.ClientSecret, nil
}

// GetPaymentMethodFromSetupIntent retrieves the payment method attached after confirmation.
func (s *Service) GetPaymentMethodFromSetupIntent(setupIntentID string) (string, error) {
	si, err := setupintent.Get(setupIntentID, nil)
	if err != nil {
		return "", fmt.Errorf("get setup intent: %w", err)
	}
	if si.PaymentMethod == nil {
		return "", fmt.Errorf("setup intent has no payment method")
	}
	return si.PaymentMethod.ID, nil
}

// ChargePaymentMethod creates an off-session PaymentIntent and confirms it immediately.
func (s *Service) ChargePaymentMethod(pmID string, amountCents int64, currency, description string) (string, error) {
	pm, err := paymentmethod.Get(pmID, nil)
	if err != nil {
		return "", fmt.Errorf("get payment method: %w", err)
	}

	piParams := &stripe.PaymentIntentParams{
		Amount:        stripe.Int64(amountCents),
		Currency:      stripe.String(currency),
		PaymentMethod: stripe.String(pmID),
		Customer:      stripe.String(pm.Customer.ID),
		Description:   stripe.String(description),
		Confirm:       stripe.Bool(true),
		OffSession:    stripe.Bool(true),
	}
	pi, err := paymentintent.New(piParams)
	if err != nil {
		return "", fmt.Errorf("create payment intent: %w", err)
	}
	return pi.ID, nil
}

// DetachPaymentMethod detaches a saved card (e.g. on cancellation).
func (s *Service) DetachPaymentMethod(pmID string) error {
	if pmID == "" {
		return nil
	}
	_, err := paymentmethod.Detach(pmID, nil)
	return err
}
