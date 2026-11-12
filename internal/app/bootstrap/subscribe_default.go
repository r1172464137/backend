package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/perfect-panel/server/internal/module/platform/entity/client"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
)

var subscribeOutputFormats = map[string]bool{"base64": true, "yaml": true, "json": true, "conf": true}

// repairDefaultApplication enforces exactly one default subscribe application
// (MIGRATION 5.2). It is idempotent and stops startup when the choice is not
// unique, instead of picking by list order.
func repairDefaultApplication(ctx context.Context, store repository.Store) error {
	apps, err := store.Client().List(ctx)
	if err != nil {
		return err
	}
	var defaults []*client.SubscribeApplication
	for _, a := range apps {
		if !subscribeOutputFormats[a.OutputFormat] {
			return fmt.Errorf("subscribe application %d %q has unsupported output_format %q", a.Id, a.Name, a.OutputFormat)
		}
		if a.IsDefault {
			defaults = append(defaults, a)
		}
	}
	if len(apps) == 0 || len(defaults) == 1 {
		return nil
	}
	keep := chooseDefaultApplication(apps, defaults)
	if keep != nil {
		logger.Infof("[Migrate] subscribe default repair: keeping application %d %q (%s, ua %q) as the only default; %d defaults before",
			keep.Id, keep.Name, keep.OutputFormat, keep.UserAgent, len(defaults))
		repaired := *keep
		repaired.IsDefault = true
		return store.Client().Update(ctx, &repaired)
	}
	var summary []string
	for _, a := range apps {
		summary = append(summary, fmt.Sprintf("%d %q %s default=%t", a.Id, a.Name, a.OutputFormat, a.IsDefault))
	}
	return fmt.Errorf("cannot choose a unique default subscribe application; set one manually: %s", strings.Join(summary, "; "))
}

// chooseDefaultApplication applies the MIGRATION 5.2 preference order and
// returns nil when no rule yields exactly one base64 application.
func chooseDefaultApplication(apps, defaults []*client.SubscribeApplication) *client.SubscribeApplication {
	has := func(a *client.SubscribeApplication, word string) bool {
		return strings.Contains(strings.ToLower(a.Name), word) || strings.Contains(strings.ToLower(a.UserAgent), word)
	}
	rules := []struct {
		from []*client.SubscribeApplication
		ok   func(*client.SubscribeApplication) bool
	}{
		{defaults, func(a *client.SubscribeApplication) bool { return strings.EqualFold(a.Name, "Default") }},
		{defaults, func(a *client.SubscribeApplication) bool { return has(a, "v2ray") }},
		{apps, func(a *client.SubscribeApplication) bool { return has(a, "default") || has(a, "v2ray") }},
		{apps, func(*client.SubscribeApplication) bool { return true }},
	}
	for _, rule := range rules {
		var hit []*client.SubscribeApplication
		for _, a := range rule.from {
			if a.OutputFormat == "base64" && rule.ok(a) {
				hit = append(hit, a)
			}
		}
		if len(hit) == 1 {
			return hit[0]
		}
	}
	return nil
}
