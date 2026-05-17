package stripe

// Client is the Stripe surface used by the API and scheduler (mockable in tests).
type Client interface {
	CreateSetupIntent(email string) (setupIntentID, clientSecret string, err error)
	GetPaymentMethodFromSetupIntent(setupIntentID string) (string, error)
	ChargePaymentMethod(pmID string, amountCents int64, currency, description, idempotencyKey string) (piID, receiptURL string, err error)
	DetachPaymentMethod(pmID string) error
}
