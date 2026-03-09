// Service layer for payment operations.
// CreateCheckout: creates a Stripe Checkout session and returns the URL.
// HandleWebhookEvent: processes Stripe events, creates sessions on checkout.session.completed.
// Receives a SessionCreator interface to create sessions (injected from main.go).
package payment
