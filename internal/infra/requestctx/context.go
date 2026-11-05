// Package requestctx defines the keys under which the HTTP middleware stores
// what it learned about a request (the signed-in user, the session, the
// payment method of a callback) in the request context. The keys live here,
// apart from the middleware, so that the modules reading the values need not
// import the transport layer.
package requestctx

// CtxKey is the type of the request context keys; a distinct type keeps
// them from colliding with keys of other packages.
type CtxKey string

const (
	// CtxKeyUser holds the signed-in user.
	CtxKeyUser CtxKey = "user"
	// CtxKeySessionID holds the session ID of the access token.
	CtxKeySessionID CtxKey = "sessionId"
	// CtxKeyRequestHost holds the Host the request was sent to.
	CtxKeyRequestHost CtxKey = "requestHost"
	// CtxKeyPlatform and CtxKeyPayment hold the platform and the payment
	// method a payment callback is for.
	CtxKeyPlatform CtxKey = "platform"
	CtxKeyPayment  CtxKey = "payment"
	// CtxKeyDeviceSecure marks a request that came over the encrypted
	// device transport.
	CtxKeyDeviceSecure CtxKey = "deviceSecure"
	// LoginType holds how the caller signed in, such as "device".
	LoginType CtxKey = "loginType"
)
