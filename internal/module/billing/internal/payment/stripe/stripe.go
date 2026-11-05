// Package stripe implements the Stripe payment protocol: payment intents and
// their payment sheets, customers, webhook endpoints and signed webhook
// events.
package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/client"
	"github.com/stripe/stripe-go/v81/webhook"
)

const APIVersion = "2024-04-10"

type Config struct {
	PublicKey     string
	SecretKey     string
	WebhookSecret string
	// Backends overrides the Stripe API endpoints; tests point them at a
	// local fake. Nil selects the SDK's shared production backends.
	Backends *stripe.Backends
}

type User struct {
	UserId int64
	Email  string
}
type NotifyResult struct {
	EventType string
	OrderNo   string
	TradeNo   string
	Method    string
	UserId    int64
	Amount    int64
	Currency  string
}

// Order is a payment to collect. Amount is in hundredths of Currency (see
// units.go for how it maps onto Stripe's unit).
type Order struct {
	OrderNo   string
	Subscribe string
	Amount    int64
	Currency  string
	Payment   string
}

// Client talks to Stripe with its own secret key. Every request carries the
// key of the payment method it belongs to; nothing is written to the SDK's
// process-wide default key, so several Stripe methods can serve concurrent
// requests without borrowing each other's account.
type Client struct {
	Config
	api *client.API
}

type PaymentSheet struct {
	ClientSecret   string
	EphemeralKey   string
	Customer       string
	PublishableKey string
	TradeNo        string
}

func NewClient(config Config) *Client {
	return &Client{
		Config: config,
		api:    client.New(config.SecretKey, config.Backends),
	}
}

func (c *Client) CreatePaymentSheet(ctx context.Context, order *Order, user *User) (*PaymentSheet, error) {
	if order == nil || order.OrderNo == "" || order.Amount < 0 || order.Currency == "" || order.Payment == "" {
		return nil, errors.New("invalid Stripe order")
	}
	var customerDataRes *stripe.Customer
	var err error
	var userID int64
	if user != nil {
		userID = user.UserId
	}
	// A guest checkout has no stable Stripe customer identity.  Do not reuse
	// the synthetic user_id=0 customer across unrelated buyers.
	if user != nil && (user.Email != "" || user.UserId != 0) {
		customerDataRes, err = c.SearchStripeCustomer(ctx, user)
		if err != nil {
			return nil, err
		}
		if customerDataRes == nil {
			customerDataRes, err = c.CreateCustomer(ctx, user)
			if err != nil {
				return nil, err
			}
		}
	}
	// Create Payment Intent
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(ToStripeAmount(order.Amount, order.Currency)),
		Currency: stripe.String(strings.ToLower(order.Currency)),
		PaymentMethodTypes: []*string{
			stripe.String(order.Payment),
		},
		Metadata: map[string]string{
			"order_no":  order.OrderNo,
			"user_id":   strconv.FormatInt(userID, 10),
			"subscribe": order.Subscribe,
		},
	}
	params.Context = ctx
	if customerDataRes != nil {
		params.Customer = stripe.String(customerDataRes.ID)
	}
	// Retrying the checkout after a network timeout must return the same
	// PaymentIntent rather than creating another chargeable transaction.
	params.SetIdempotencyKey("ppanel:payment-intent:" + order.OrderNo)
	result, err := c.api.PaymentIntents.New(params)
	if err != nil {
		return nil, err
	}
	sheet := &PaymentSheet{
		ClientSecret:   result.ClientSecret,
		PublishableKey: c.PublicKey,
		TradeNo:        result.ID,
	}
	if customerDataRes != nil {
		// Preserve the original mobile-SDK support for identified users.  Guest
		// checkouts intentionally have no customer or ephemeral key.
		ekParams := &stripe.EphemeralKeyParams{
			Customer:      stripe.String(customerDataRes.ID),
			StripeVersion: stripe.String(APIVersion),
		}
		ekParams.Context = ctx
		ek, err := c.api.EphemeralKeys.New(ekParams)
		if err != nil {
			return nil, err
		}
		sheet.EphemeralKey = ek.Secret
		sheet.Customer = customerDataRes.ID
	}
	return sheet, nil
}

// GetPaymentSheet returns the already-created PaymentIntent for a repeat
// checkout.  It validates the immutable fields recorded in Stripe before
// exposing its client secret again.
func (c *Client) GetPaymentSheet(ctx context.Context, order *Order, tradeNo string) (*PaymentSheet, error) {
	intent, err := c.matchingIntent(ctx, order, tradeNo)
	if err != nil {
		return nil, err
	}
	if intent.Status == stripe.PaymentIntentStatusCanceled {
		return nil, errors.New("stored Stripe payment intent is canceled")
	}
	return &PaymentSheet{
		ClientSecret:   intent.ClientSecret,
		PublishableKey: c.PublicKey,
		TradeNo:        intent.ID,
	}, nil
}

// matchingIntent loads the PaymentIntent and checks that it was created for
// order: same order number, amount, currency and single payment method.
func (c *Client) matchingIntent(ctx context.Context, order *Order, tradeNo string) (*stripe.PaymentIntent, error) {
	if order == nil || tradeNo == "" {
		return nil, errors.New("invalid Stripe payment intent lookup")
	}
	intent, err := c.getIntent(ctx, tradeNo)
	if err != nil {
		return nil, err
	}
	if intent.Metadata["order_no"] != order.OrderNo || intent.Amount != ToStripeAmount(order.Amount, order.Currency) ||
		!strings.EqualFold(string(intent.Currency), order.Currency) ||
		len(intent.PaymentMethodTypes) != 1 || intent.PaymentMethodTypes[0] != order.Payment {
		return nil, errors.New("stored Stripe payment intent does not match order")
	}
	return intent, nil
}

func (c *Client) getIntent(ctx context.Context, tradeNo string) (*stripe.PaymentIntent, error) {
	params := &stripe.PaymentIntentParams{}
	params.Context = ctx
	return c.api.PaymentIntents.Get(tradeNo, params)
}

// SearchStripeCustomer  Search for a Stripe customer by email or user ID
func (c *Client) SearchStripeCustomer(ctx context.Context, user *User) (*stripe.Customer, error) {
	params := &stripe.CustomerSearchParams{}
	params.Context = ctx
	if user.Email != "" {
		params.Query = fmt.Sprintf("email:'%s'", user.Email)
	} else {
		params.Query = fmt.Sprintf("metadata['user_id']:'%d'", user.UserId)
	}
	result := c.api.Customers.Search(params)
	if result.Err() != nil {
		return nil, result.Err()
	}
	if len(result.CustomerSearchResult().Data) != 0 {
		return result.CustomerSearchResult().Data[0], nil
	}
	return nil, nil
}

// CreateCustomer Create a new Stripe customer
func (c *Client) CreateCustomer(ctx context.Context, user *User) (*stripe.Customer, error) {
	customerData := &stripe.CustomerParams{}
	customerData.Context = ctx
	if user.Email != "" {
		customerData.Email = &user.Email
	}
	customerData.AddMetadata("user_id", strconv.FormatInt(user.UserId, 10))
	return c.api.Customers.New(customerData)
}

// QueryOrderStatus reports whether the PaymentIntent has succeeded.
func (c *Client) QueryOrderStatus(ctx context.Context, tradeNo string) (bool, error) {
	intent, err := c.getIntent(ctx, tradeNo)
	if err != nil {
		return false, err
	}
	return intent.Status == stripe.PaymentIntentStatusSucceeded, nil
}

// VerifyPaymentIntent checks that the stored intent still belongs to the
// order before returning its payment state. It is used by expiry handling so
// a successful intent can be settled instead of being closed locally.
func (c *Client) VerifyPaymentIntent(ctx context.Context, order *Order, tradeNo string) (bool, error) {
	intent, err := c.matchingIntent(ctx, order, tradeNo)
	if err != nil {
		return false, err
	}
	return intent.Status == stripe.PaymentIntentStatusSucceeded, nil
}

// PaymentIntentStatus returns the status of the order's intent after
// checking, like VerifyPaymentIntent, that the intent still belongs to the
// order.
func (c *Client) PaymentIntentStatus(ctx context.Context, order *Order, tradeNo string) (stripe.PaymentIntentStatus, error) {
	intent, err := c.matchingIntent(ctx, order, tradeNo)
	if err != nil {
		return "", err
	}
	return intent.Status, nil
}

// CancelPaymentIntent prevents a still-pending client secret from being paid
// after the local order has expired.
func (c *Client) CancelPaymentIntent(ctx context.Context, tradeNo string) error {
	if tradeNo == "" {
		return errors.New("stripe payment intent is missing")
	}
	params := &stripe.PaymentIntentCancelParams{}
	params.Context = ctx
	_, err := c.api.PaymentIntents.Cancel(tradeNo, params)
	return err
}

// ParseNotify authenticates a webhook payload with the endpoint secret and
// extracts the PaymentIntent it reports.
func (c *Client) ParseNotify(payload []byte, signature string) (*NotifyResult, error) {
	event, err := webhook.ConstructEventWithOptions(payload, signature, c.WebhookSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		return nil, err
	}
	var paymentIntent stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &paymentIntent); err != nil {
		return nil, fmt.Errorf("decode Stripe payment intent: %w", err)
	}
	var method string
	if len(paymentIntent.PaymentMethodTypes) > 0 {
		method = paymentIntent.PaymentMethodTypes[0]
	}
	uid, _ := strconv.ParseInt(paymentIntent.Metadata["user_id"], 10, 64)
	return &NotifyResult{
		EventType: string(event.Type),
		OrderNo:   paymentIntent.Metadata["order_no"],
		TradeNo:   paymentIntent.ID,
		UserId:    uid,
		Method:    method,
		Amount:    FromStripeAmount(paymentIntent.AmountReceived, string(paymentIntent.Currency)),
		Currency:  string(paymentIntent.Currency),
	}, nil
}

// CreateWebhookEndpoint registers url for the payment events the callback
// handler settles and returns the endpoint with its signing secret.
func (c *Client) CreateWebhookEndpoint(ctx context.Context, url string) (*stripe.WebhookEndpoint, error) {
	params := &stripe.WebhookEndpointParams{
		URL: stripe.String(url),
		EnabledEvents: []*string{
			stripe.String("payment_intent.succeeded"),
			stripe.String("payment_intent.payment_failed"),
		},
	}
	params.Context = ctx
	return c.api.WebhookEndpoints.New(params)
}

// DeleteWebhookEndpoint removes a webhook endpoint; it undoes an endpoint
// whose payment method could not be saved.
func (c *Client) DeleteWebhookEndpoint(ctx context.Context, id string) error {
	params := &stripe.WebhookEndpointParams{}
	params.Context = ctx
	_, err := c.api.WebhookEndpoints.Del(id, params)
	return err
}
