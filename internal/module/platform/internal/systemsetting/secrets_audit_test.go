package systemsetting

import (
	"context"
	"errors"
	"strings"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/pkg/requestmeta"
	"github.com/perfect-panel/server/pkg/xerr"
)

// adminContext is an administrator's request: address, agent and account 9.
func adminContext() context.Context {
	return requestmeta.WithActor(requestmeta.With(context.Background(), requestmeta.New("203.0.113.9", "AdminPanel/1.0")), 9)
}

// actions reads the administrator actions recorded so far, oldest first.
func (w *settingsWorld) actions(t *testing.T) []log.AdminAction {
	t.Helper()
	var rows []log.SystemLog
	if err := w.db.Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	actions := make([]log.AdminAction, 0, len(rows))
	for _, row := range rows {
		if row.Type != log.TypeAdminAction.Uint8() || row.ObjectID != 9 {
			t.Fatalf("row %+v is not an action filed under administrator 9", row)
		}
		var action log.AdminAction
		if err := action.Unmarshal([]byte(row.Content)); err != nil {
			t.Fatal(err)
		}
		actions = append(actions, action)
	}
	return actions
}

// storedValue reads a setting as stored, in clear.
func (w *settingsWorld) storedValue(t *testing.T, category, key string) string {
	t.Helper()
	var row system.System
	if err := w.db.Where("category = ? AND `key` = ?", category, key).First(&row).Error; err != nil {
		t.Fatalf("%s.%s: %v", category, key, err)
	}
	return row.Value
}

// The secrets read back masked; an update that carries the mask keeps the
// stored value, an update that carries a value replaces it and an empty
// field clears it. Unset secrets read back empty, so the panel can tell
// "not configured" from "configured".
func TestSecretsReadBackMaskedAndAreKeptWhenMasked(t *testing.T) {
	w := newSettingsWorld(t)
	ctx := adminContext()

	if got, err := w.svc.GetVerifyConfig(ctx); err != nil || got.TurnstileSecret != "" {
		t.Fatalf("unset secret reads as %q (err %v), want empty", got.TurnstileSecret, err)
	}
	if err := w.svc.UpdateVerifyConfig(ctx, &dto.VerifyConfig{TurnstileSiteKey: "site", TurnstileSecret: "s3cret"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := w.svc.GetVerifyConfig(ctx); got.TurnstileSecret != dto.SecretMask || got.TurnstileSiteKey != "site" {
		t.Fatalf("verify = %+v, want the secret masked and the site key in clear", got)
	}
	// The panel sends the settings back as it received them.
	if err := w.svc.UpdateVerifyConfig(ctx, &dto.VerifyConfig{TurnstileSiteKey: "site-2", TurnstileSecret: dto.SecretMask, EnableLoginVerify: true}); err != nil {
		t.Fatal(err)
	}
	if got := w.storedValue(t, "verify", "TurnstileSecret"); got != "s3cret" {
		t.Fatalf("stored secret = %q, want the masked update to keep it", got)
	}
	if err := w.svc.UpdateVerifyConfig(ctx, &dto.VerifyConfig{TurnstileSiteKey: "site-2", TurnstileSecret: "n3w"}); err != nil {
		t.Fatal(err)
	}
	if got := w.storedValue(t, "verify", "TurnstileSecret"); got != "n3w" {
		t.Fatalf("stored secret = %q, want it replaced", got)
	}
	if err := w.svc.UpdateVerifyConfig(ctx, &dto.VerifyConfig{TurnstileSiteKey: "site-2"}); err != nil {
		t.Fatal(err)
	}
	if got := w.storedValue(t, "verify", "TurnstileSecret"); got != "" {
		t.Fatalf("stored secret = %q, want it cleared", got)
	}

	if err := w.svc.UpdateCurrencyConfig(ctx, &dto.CurrencyConfig{AccessKey: "ak", CurrencyUnit: "USD", CurrencySymbol: "$"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := w.svc.GetCurrencyConfig(ctx); got.AccessKey != dto.SecretMask {
		t.Fatalf("currency = %+v, want the access key masked", got)
	}
	if err := w.svc.UpdateCurrencyConfig(ctx, &dto.CurrencyConfig{AccessKey: dto.SecretMask, CurrencyUnit: "EUR", CurrencySymbol: "€"}); err != nil {
		t.Fatal(err)
	}
	if got := w.storedValue(t, "currency", "AccessKey"); got != "ak" {
		t.Fatalf("stored access key = %q, want the masked update to keep it", got)
	}
}

// A mask for a secret the store has no value of cannot be kept: the update
// is refused and nothing is written.
func TestMaskedSecretWithoutStoredValueIsRefused(t *testing.T) {
	w := newSettingsWorld(t)
	ctx := adminContext()
	for name, update := range map[string]func() error{
		"verify":   func() error { return w.svc.UpdateVerifyConfig(ctx, &dto.VerifyConfig{TurnstileSecret: dto.SecretMask}) },
		"currency": func() error { return w.svc.UpdateCurrencyConfig(ctx, &dto.CurrencyConfig{AccessKey: dto.SecretMask}) },
	} {
		if err := update(); xerr.CodeOf(err) != xerr.InvalidParams {
			t.Fatalf("%s: err = %v, want a parameter error", name, err)
		}
	}
	if calls := w.runtime.calls(); len(calls) != 0 {
		t.Fatalf("refused updates reloaded %v", calls)
	}
	if n := len(w.actions(t)); n != 0 {
		t.Fatalf("refused updates left %d audit rows", n)
	}
}

// Every settings update leaves an audit row filed under the administrator,
// naming the category and the keys whose stored value changed — never a
// value, since several categories hold secrets — and no row when nothing
// changed.
func TestSettingsUpdatesAreAuditedByChangedKey(t *testing.T) {
	w := newSettingsWorld(t)
	ctx := adminContext()
	if err := w.svc.UpdateVerifyConfig(ctx, &dto.VerifyConfig{TurnstileSiteKey: "site", TurnstileSecret: "s3cret", EnableLoginVerify: true}); err != nil {
		t.Fatal(err)
	}
	// Only the site key changes; the secret is sent back masked.
	if err := w.svc.UpdateVerifyConfig(ctx, &dto.VerifyConfig{TurnstileSiteKey: "site-2", TurnstileSecret: dto.SecretMask, EnableLoginVerify: true}); err != nil {
		t.Fatal(err)
	}
	// Nothing changes.
	if err := w.svc.UpdateVerifyConfig(ctx, &dto.VerifyConfig{TurnstileSiteKey: "site-2", TurnstileSecret: dto.SecretMask, EnableLoginVerify: true}); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.UpdateSiteConfig(ctx, &dto.SiteConfig{SiteName: "Panel"}); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.SetNodeMultiplier(ctx, &dto.SetNodeMultiplierRequest{Periods: []dto.TimePeriod{{StartTime: "00:00", EndTime: "06:00", Multiplier: 0.5}}}); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.SettingTelegramBot(ctx); err != nil {
		t.Fatal(err)
	}

	actions := w.actions(t)
	want := []struct{ action, object, detail string }{
		{"settings.update", "verify", "keys: TurnstileSiteKey, TurnstileSecret, EnableLoginVerify"},
		{"settings.update", "verify", "keys: TurnstileSiteKey"},
		{"settings.update", "site", "keys: SiteName"},
		{"settings.update", "server", "keys: NodeMultiplierConfig"},
		{"settings.reload", "telegram", ""},
	}
	if len(actions) != len(want) {
		t.Fatalf("audited %d actions, want %d: %+v", len(actions), len(want), actions)
	}
	for i, tc := range want {
		got := actions[i]
		if got.Action != tc.action || got.Object != tc.object || got.Detail != tc.detail || got.ActorID != 9 || got.ClientIP != "203.0.113.9" ||
			got.Source != log.AdminActionSourceHTTP || got.Timestamp == 0 {
			t.Fatalf("action %d = %+v, want %+v by administrator 9", i, got, tc)
		}
		if strings.Contains(got.Detail, "s3cret") || strings.Contains(got.Detail, "site-2") {
			t.Fatalf("action %d records a value: %q", i, got.Detail)
		}
	}
}

// The audit row is written in the update's transaction: a rolled-back
// update leaves no trail of a change that did not happen.
func TestRolledBackSettingsUpdateLeavesNoAuditRow(t *testing.T) {
	w := newSettingsWorld(t)
	w.tx.failCommit = errors.New("commit failed")
	if err := w.svc.UpdateSiteConfig(adminContext(), &dto.SiteConfig{SiteName: "after"}); xerr.CodeOf(err) != xerr.DatabaseUpdateError {
		t.Fatalf("err = %v, want a database update error", err)
	}
	if n := len(w.actions(t)); n != 0 {
		t.Fatalf("a rolled-back update left %d audit rows", n)
	}
}
