package oauthstate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newClient(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return server, client
}

// A state is redeemed once, only by the method it was issued for, and not
// after it expired.
func TestIssuedStateIsRedeemedOnce(t *testing.T) {
	server, client := newClient(t)
	ctx := context.Background()

	state, err := Issue(ctx, client, "google", LoginScope(), "https://panel.example/callback", "")
	if err != nil || state == "" {
		t.Fatalf("Issue() = %q, %v", state, err)
	}
	if ttl := server.TTL(key("google", state)); ttl != TTL {
		t.Fatalf("state TTL = %v, want %v", ttl, TTL)
	}
	if _, err := Consume(ctx, client, "github", LoginScope(), state, ""); !errors.Is(err, ErrUnknown) {
		t.Fatalf("another method redeemed the state: %v", err)
	}
	peeked, err := Peek(ctx, client, "google", state)
	if err != nil || peeked != "https://panel.example/callback" {
		t.Fatalf("Peek() = %q, %v", peeked, err)
	}
	redirect, err := Consume(ctx, client, "google", LoginScope(), state, "")
	if err != nil || redirect != "https://panel.example/callback" {
		t.Fatalf("Consume() = %q, %v", redirect, err)
	}
	if _, err := Consume(ctx, client, "google", LoginScope(), state, ""); !errors.Is(err, ErrUnknown) {
		t.Fatalf("a redeemed state was redeemed again: %v", err)
	}

	expiring, err := Issue(ctx, client, "apple", LoginScope(), "https://panel.example/callback", "")
	if err != nil {
		t.Fatal(err)
	}
	server.FastForward(TTL)
	if _, err := Peek(ctx, client, "apple", expiring); !errors.Is(err, ErrUnknown) {
		t.Fatalf("an expired state was found: %v", err)
	}
}

// A state belongs to the flow that issued it: a sign-in state cannot
// complete a binding, a binding state cannot complete a sign-in or another
// account's binding, and a state redeemed out of scope is spent.
func TestStateIsRedeemedOnlyInItsScope(t *testing.T) {
	_, client := newClient(t)
	ctx := context.Background()

	login, err := Issue(ctx, client, "github", LoginScope(), "https://panel.example/login", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Consume(ctx, client, "github", BindScope(7), login, ""); !errors.Is(err, ErrScope) {
		t.Fatalf("a binding redeemed a sign-in state: %v", err)
	}
	if _, err := Consume(ctx, client, "github", LoginScope(), login, ""); !errors.Is(err, ErrUnknown) {
		t.Fatalf("a state redeemed out of scope was still there: %v", err)
	}

	bind, err := Issue(ctx, client, "github", BindScope(7), "https://panel.example/bind", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Consume(ctx, client, "github", LoginScope(), bind, ""); !errors.Is(err, ErrScope) {
		t.Fatalf("a sign-in redeemed a binding state: %v", err)
	}
	bind, err = Issue(ctx, client, "github", BindScope(7), "https://panel.example/bind", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Consume(ctx, client, "github", BindScope(8), bind, ""); !errors.Is(err, ErrScope) {
		t.Fatalf("another account redeemed a binding state: %v", err)
	}
	bind, err = Issue(ctx, client, "github", BindScope(7), "https://panel.example/bind", "")
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := Consume(ctx, client, "github", BindScope(7), bind, "")
	if err != nil || redirect != "https://panel.example/bind" {
		t.Fatalf("Consume() by the issuing account = %q, %v", redirect, err)
	}
	if peeked, err := Peek(ctx, client, "github", login); err == nil || peeked != "" {
		t.Fatalf("Peek() of a spent state = %q, %v", peeked, err)
	}
}

// A state issued with a nonce belongs to the client that chose it: it is
// redeemed with the same nonce only, so a sign-in an attacker started in
// their browser is not completed by a victim's browser, which has no nonce
// or another. A state issued without a nonce is refused when one is
// presented, since the presenting client did not start that sign-in. The
// nonce itself is never stored, only its digest. A refused state is spent.
func TestStateIsRedeemedOnlyWithItsNonce(t *testing.T) {
	server, client := newClient(t)
	ctx := context.Background()

	bound, err := Issue(ctx, client, "github", LoginScope(), "https://panel.example/login", "client-nonce-0123456789")
	if err != nil {
		t.Fatal(err)
	}
	if stored, _ := server.Get(key("github", bound)); strings.Contains(stored, "client-nonce") || !strings.Contains(stored, `"nonce_hash":"`) {
		t.Fatalf("stored state = %q, want the nonce's digest and not the nonce", stored)
	}
	for name, nonce := range map[string]string{"no nonce": "", "another nonce": "attacker-nonce-0123456789"} {
		if _, err := Consume(ctx, client, "github", LoginScope(), bound, nonce); !errors.Is(err, ErrNonce) {
			t.Fatalf("%s redeemed a bound state: %v", name, err)
		}
		if _, err := Consume(ctx, client, "github", LoginScope(), bound, "client-nonce-0123456789"); !errors.Is(err, ErrUnknown) {
			t.Fatalf("a state refused for its nonce was still there: %v", err)
		}
		if bound, err = Issue(ctx, client, "github", LoginScope(), "https://panel.example/login", "client-nonce-0123456789"); err != nil {
			t.Fatal(err)
		}
	}
	redirect, err := Consume(ctx, client, "github", LoginScope(), bound, "client-nonce-0123456789")
	if err != nil || redirect != "https://panel.example/login" {
		t.Fatalf("Consume() with the nonce = %q, %v", redirect, err)
	}

	unbound, err := Issue(ctx, client, "github", LoginScope(), "https://panel.example/login", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Consume(ctx, client, "github", LoginScope(), unbound, "victim-nonce-0123456789"); !errors.Is(err, ErrNonce) {
		t.Fatalf("a nonce redeemed a state issued without one: %v", err)
	}
	unbound, err = Issue(ctx, client, "github", LoginScope(), "https://panel.example/login", "")
	if err != nil {
		t.Fatal(err)
	}
	if redirect, err := Consume(ctx, client, "github", LoginScope(), unbound, ""); err != nil || redirect != "https://panel.example/login" {
		t.Fatalf("Consume() of a state without a nonce = %q, %v", redirect, err)
	}
}

// A state stored without a scope, by a version before scopes, is not trusted
// as any scope.
func TestStateWithoutAScopeIsUnknown(t *testing.T) {
	server, client := newClient(t)
	if err := server.Set(key("github", "legacy"), "https://panel.example/callback"); err != nil {
		t.Fatal(err)
	}
	if _, err := Peek(context.Background(), client, "github", "legacy"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Peek() = %v, want ErrUnknown", err)
	}
	if _, err := Consume(context.Background(), client, "github", LoginScope(), "legacy", ""); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Consume() = %v, want ErrUnknown", err)
	}
}

func TestIssuedStatesAreDistinct(t *testing.T) {
	server, client := newClient(t)
	first, err := Issue(context.Background(), client, "google", LoginScope(), "https://panel.example/a", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Issue(context.Background(), client, "google", BindScope(7), "https://panel.example/b", "")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two round trips got the same state")
	}
	// The scope travels with the state, not in the key the callback names.
	if stored, _ := server.Get(key("google", second)); !strings.Contains(stored, `"user_id":7`) || !strings.Contains(stored, `"purpose":"bind"`) {
		t.Fatalf("stored state = %q, want the binding's account and purpose", stored)
	}
}
