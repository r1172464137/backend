package systemsetting

import (
	"errors"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/xerr"
)

// Verification and currency settings mask their stored secrets. A masked
// update preserves the old value. Ordered routing and the node credential
// instead use the authorized administrator's full-package read contract.

// maskSecret shows a stored secret masked; an unset secret stays empty, so
// the admin panel can tell "not configured" from "configured".
func maskSecret(value string) string {
	if value == "" {
		return ""
	}
	return dto.SecretMask
}

// errMaskedSecretUnknown refuses an update that carries the mask for a
// secret the store has no value of, which the mask cannot stand in for.
var errMaskedSecretUnknown = errors.New("masked secret has no stored value")

func maskedSecretUnknown(what string) error {
	return xerr.Wrapf(errMaskedSecretUnknown, xerr.InvalidParams, "%s is masked but no stored value exists to keep", what)
}

// maskVerifySecrets masks the Turnstile secret of the verification
// settings.
func maskVerifySecrets(c *dto.VerifyConfig) {
	c.TurnstileSecret = maskSecret(c.TurnstileSecret)
}

// keepVerifySecrets resolves a masked Turnstile secret from the stored
// settings; nil stored settings are unknown.
func keepVerifySecrets(req *dto.VerifyConfig, stored *dto.VerifyConfig) error {
	if req.TurnstileSecret != dto.SecretMask {
		return nil
	}
	if stored == nil || stored.TurnstileSecret == "" {
		return maskedSecretUnknown("turnstile_secret")
	}
	req.TurnstileSecret = stored.TurnstileSecret
	return nil
}

// maskCurrencySecrets masks the exchange-rate provider's access key.
func maskCurrencySecrets(c *dto.CurrencyConfig) {
	c.AccessKey = maskSecret(c.AccessKey)
}

// keepCurrencySecrets resolves a masked access key from the stored settings.
func keepCurrencySecrets(req *dto.CurrencyConfig, stored *dto.CurrencyConfig) error {
	if req.AccessKey != dto.SecretMask {
		return nil
	}
	if stored == nil || stored.AccessKey == "" {
		return maskedSecretUnknown("access_key")
	}
	req.AccessKey = stored.AccessKey
	return nil
}
