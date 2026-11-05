package integration

import (
	"errors"
	"fmt"
	"net/url"
)

// RequestError reports a failed provider call, prefixed with the provider's
// name and without the request URL. net/http reports a failed round trip as
// a *url.Error that prints the whole URL, and a provider's URL can carry the
// account, a password hash, the recipient's number or the code it delivers;
// the sending tasks log the error, so only the underlying failure is kept.
func RequestError(provider string, err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	return fmt.Errorf("%s request: %w", provider, err)
}
