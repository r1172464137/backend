// Package oauthprovider defines the OAuth sign-in methods: a Provider builds
// the authorization URL of one method and turns its callback into the
// identity the provider vouches for. Registry lists the methods and how
// their round trip runs; the subpackages hold the provider API clients.
package oauthprovider

import (
	"context"
	"errors"
)

// Identity is the account a provider vouches for after a callback.
type Identity struct {
	// Subject is the provider's stable user id, stored as the identifier.
	Subject string
	// Email is an address the provider verified, or empty.
	Email  string
	Avatar string
	// ReplayKey is set for a callback that is a bearer credential without
	// a state round trip (Telegram): the flow redeems the key once.
	ReplayKey string
}

// Callback is what the browser brought back from the provider.
type Callback struct {
	// Code and Redirect belong to state-based methods: the authorization
	// code and the redirect the state was issued for.
	Code     string
	Redirect string
	// Fields is the raw callback object.
	Fields map[string]any
}

// Provider is one OAuth method, built from its stored configuration.
type Provider interface {
	// AuthURL returns the URL starting a sign-in that comes back to
	// redirect. state is empty for methods without a state round trip.
	AuthURL(redirect, state string) (string, error)
	// Identify completes the sign-in the callback belongs to. The context
	// bounds the provider requests.
	Identify(ctx context.Context, callback Callback) (*Identity, error)
}

// Method describes an OAuth method.
type Method struct {
	// State reports that the callback returns a state issued with the
	// authorization URL.
	State bool
	// PinRedirect keeps the redirect on the configured site host: the
	// provider sends a credential to it or it becomes a browser redirect.
	PinRedirect bool
	// New builds the provider from the method's stored JSON configuration.
	New func(config string) (Provider, error)
}

// Registry maps method names to their methods.
type Registry map[string]Method

// Lookup returns the method called name.
func (r Registry) Lookup(name string) (Method, bool) {
	method, ok := r[name]
	return method, ok
}

// The errors a provider reports; the flow turns them into client codes.
var (
	// ErrMisconfigured reports a stored configuration the provider cannot
	// work with.
	ErrMisconfigured = errors.New("oauth provider is misconfigured")
	// ErrInvalidCallback reports a callback that is malformed or whose
	// signature does not verify.
	ErrInvalidCallback = errors.New("oauth callback is invalid")
	// ErrExpiredCallback reports a signed callback older than its window.
	ErrExpiredCallback = errors.New("oauth callback has expired")
	// ErrRejected reports a provider refusing the sign-in or failing.
	ErrRejected = errors.New("oauth provider rejected the sign-in")
)

// Default returns the built-in methods.
func Default() Registry {
	return Registry{
		"google":   {State: true, New: newGoogle},
		"github":   {State: true, New: newGithub},
		"facebook": {State: true, New: newFacebook},
		// The Apple form-post callback redirects the browser to the stored
		// redirect, so it stays on the site host.
		"apple": {State: true, PinRedirect: true, New: newApple},
		// Telegram sends the signed widget result to the redirect; the
		// URL list registered with BotFather is not relied upon alone.
		"telegram": {PinRedirect: true, New: newTelegram},
	}
}
