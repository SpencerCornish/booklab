package stripe

import (
	"fmt"
	"log/slog"

	"github.com/stripe/stripe-go/v85"
	"github.com/stripe/stripe-go/v85/customer"
	"github.com/stripe/stripe-go/v85/paymentintent"
	"github.com/stripe/stripe-go/v85/paymentmethod"
	"github.com/stripe/stripe-go/v85/setupintent"
)

type Service struct {
	logger *slog.Logger
}

func New(secretKey string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	stripe.Key = secretKey
	return &Service{logger: log.With("component", "stripe")}
}

// CreateSetupIntent creates a SetupIntent for saving a card without charging.
func (s *Service) CreateSetupIntent(email string) (string, string, error) {
	s.logger.Info("stripe create_setup_intent start", "email", email)
	// Create or find a customer
	cParams := &stripe.CustomerParams{
		Email: stripe.String(email),
	}
	c, err := customer.New(cParams)
	if err != nil {
		s.logger.Error("stripe create customer failed", "email", email, "error", err)
		return "", "", fmt.Errorf("create customer: %w", err)
	}

	siParams := &stripe.SetupIntentParams{
		Customer:           stripe.String(c.ID),
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
	}
	si, err := setupintent.New(siParams)
	if err != nil {
		s.logger.Error("stripe create setup intent failed", "email", email, "customer_id", c.ID, "error", err)
		return "", "", fmt.Errorf("create setup intent: %w", err)
	}
	s.logger.Info("stripe create_setup_intent ok", "email", email, "customer_id", c.ID, "setup_intent_id", si.ID)
	return si.ID, si.ClientSecret, nil
}

// GetPaymentMethodFromSetupIntent retrieves the payment method attached after confirmation.
func (s *Service) GetPaymentMethodFromSetupIntent(setupIntentID string) (string, error) {
	s.logger.Info("stripe get_payment_method_from_setup_intent start", "setup_intent_id", setupIntentID)
	si, err := setupintent.Get(setupIntentID, nil)
	if err != nil {
		s.logger.Error("stripe get setup intent failed", "setup_intent_id", setupIntentID, "error", err)
		return "", fmt.Errorf("get setup intent: %w", err)
	}
	if si.PaymentMethod == nil {
		s.logger.Warn("stripe setup intent has no payment method", "setup_intent_id", setupIntentID)
		return "", fmt.Errorf("setup intent has no payment method")
	}
	s.logger.Info("stripe get_payment_method_from_setup_intent ok", "setup_intent_id", setupIntentID, "payment_method_id", si.PaymentMethod.ID)
	return si.PaymentMethod.ID, nil
}

// ChargePaymentMethod creates an off-session PaymentIntent and confirms it immediately.
// Returns the PaymentIntent ID and Stripe-hosted receipt URL.
// idempotencyKey must be stable for the same logical charge (e.g. per booking + charge kind).
func (s *Service) ChargePaymentMethod(pmID string, amountCents int64, currency, description, idempotencyKey string) (piID, receiptURL string, err error) {
	s.logger.Info("stripe charge_payment_method start",
		"payment_method_id", pmID, "amount_cents", amountCents, "currency", currency, "idempotency_key", idempotencyKey)
	pm, err := paymentmethod.Get(pmID, nil)
	if err != nil {
		s.logger.Error("stripe get payment method failed", "payment_method_id", pmID, "error", err)
		return "", "", fmt.Errorf("get payment method: %w", err)
	}

	piParams := &stripe.PaymentIntentParams{
		Params: stripe.Params{
			IdempotencyKey: stripe.String(idempotencyKey),
		},
		Amount:        stripe.Int64(amountCents),
		Currency:      stripe.String(currency),
		PaymentMethod: stripe.String(pmID),
		Customer:      stripe.String(pm.Customer.ID),
		Description:   stripe.String(description),
		Confirm:       stripe.Bool(true),
		OffSession:    stripe.Bool(true),
	}
	piParams.AddExpand("latest_charge")
	pi, err := paymentintent.New(piParams)
	if err != nil {
		s.logger.Error("stripe create payment intent failed",
			"payment_method_id", pmID, "customer_id", pm.Customer.ID, "amount_cents", amountCents, "idempotency_key", idempotencyKey, "error", err)
		return "", "", fmt.Errorf("create payment intent: %w", err)
	}
	if pi.LatestCharge != nil {
		receiptURL = pi.LatestCharge.ReceiptURL
	}
	s.logger.Info("stripe charge_payment_method ok", "payment_intent_id", pi.ID, "payment_method_id", pmID, "idempotency_key", idempotencyKey)
	return pi.ID, receiptURL, nil
}

// DetachPaymentMethod detaches a saved card (e.g. on cancellation).
func (s *Service) DetachPaymentMethod(pmID string) error {
	if pmID == "" {
		return nil
	}
	s.logger.Info("stripe detach_payment_method start", "payment_method_id", pmID)
	_, err := paymentmethod.Detach(pmID, nil)
	if err != nil {
		s.logger.Error("stripe detach payment method failed", "payment_method_id", pmID, "error", err)
		return err
	}
	s.logger.Info("stripe detach_payment_method ok", "payment_method_id", pmID)
	return nil
}
