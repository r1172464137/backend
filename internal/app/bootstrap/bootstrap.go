package bootstrap

import (
	"context"
	"errors"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/perfect-panel/server/internal/app/migration/schema"
	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/orm"
)

// Dependencies is the startup/reconfiguration boundary. It owns only mutable
// runtime configuration and the services needed to load or publish it.
type Dependencies struct {
	Config                   func() config.Config
	UpdateConfig             func(func(*config.Config))
	Store                    repository.Store
	ExchangeRate             *billing.CurrencyRateCache
	Notification             notification.Service
	SetTelegramBot           func(*tgbot.Bot)
	SetNodeMultiplierManager func(*network.MultiplierManager)
	FinalizeStagedAdmin      func(expectedHash string) error
}

func (d *Dependencies) currentConfig() config.Config {
	if d == nil || d.Config == nil {
		return config.Config{}
	}
	return d.Config()
}

func (d *Dependencies) updateConfig(update func(*config.Config)) {
	if d != nil && d.UpdateConfig != nil {
		d.UpdateConfig(update)
	}
}

// Start loads startup state in dependency order. Migration and node-secret
// provisioning must precede every node configuration read.
func Start(deps *Dependencies) {
	Migrate(deps)
	Site(deps)
	NodeSecret(deps)
	Node(deps)
	Email(deps)
	Device(deps)
	Invite(deps)
	Verify(deps)
	Subscribe(deps)
	Register(deps)
	Mobile(deps)
	Currency(deps)
	Telegram(deps)
}

// Reload refreshes the subsystem changed by an administrator. Startup-only
// migration and node-secret provisioning are deliberately excluded.
func Reload(deps *Dependencies, subsystem string) {
	switch subsystem {
	case "verify":
		Verify(deps)
	case "node":
		Node(deps)
	case "telegram":
		Telegram(deps)
	case "currency":
		Currency(deps)
	case "register":
		Register(deps)
	case "site":
		Site(deps)
	case "invite":
		Invite(deps)
	case "subscribe":
		Subscribe(deps)
	case "email":
		Email(deps)
	case "mobile":
		Mobile(deps)
	case "device":
		Device(deps)
	}
}

func Migrate(ctx *Dependencies) {
	current := ctx.currentConfig()
	mc := orm.Mysql{
		Config: current.DatabaseConfig(),
	}
	version, exists, recoverFirst, err := schemaVersionState(context.Background(), current.DatabaseConfig())
	if err != nil {
		panic(err)
	}
	if exists && (version < orderedMigrationVersion-1 || version > orderedMigrationVersion) {
		panic("ordered routing cutover requires schema 02179; restore backup or upgrade to 02179 first")
	}
	var plan *orderedCutoverPlan
	if !exists || version == orderedMigrationVersion-1 {
		plan, err = preflightOrderedCutover(context.Background(), ctx.Store, !exists)
	} else {
		entries, readErr := ctx.Store.System().GetNodeConfigUncached(context.Background())
		if readErr != nil {
			panic(readErr)
		}
		marker := ""
		for _, entry := range entries {
			if entry.Key == "RoutingCutover" {
				marker = entry.Value
			}
		}
		switch marker {
		case "pending":
			plan, err = preflightOrderedCutover(context.Background(), ctx.Store, false)
		case "complete":
		default:
			panic("missing or invalid ordered routing cutover marker; restore backup")
		}
	}
	if err != nil {
		panic(err)
	}
	// golang-migrate may have recorded a dirty first version before issuing
	// its first DDL. Only the strictly empty case above can reset that marker.
	if recoverFirst {
		client := schema.Migrate(mc.Driver(), mc.MigrationDsn())
		resetErr := client.Force(-1)
		sourceErr, databaseErr := client.Close()
		if err := errors.Join(resetErr, sourceErr, databaseErr); err != nil {
			panic(err)
		}
	}
	now := time.Now()
	migrationErr := schema.Up(mc.Driver(), mc.MigrationDsn())
	if migrationErr != nil && !errors.Is(migrationErr, schema.NoChange) {
		logger.Errorf("[Migrate] Up error: %v", migrationErr.Error())
		panic(migrationErr)
	}
	if plan != nil {
		if err := applyOrderedCutover(context.Background(), ctx.Store, plan); err != nil {
			panic(err)
		}
	}
	if errors.Is(migrationErr, schema.NoChange) {
		logger.Info("[Migrate] database not change")
	} else {
		logger.Info("[Migrate] Database change, took " + time.Since(now).String())
	}
	// A crash after schema.Up but before this transaction must not permanently
	// skip initial admin provisioning on the subsequent no-change startup.
	err = ctx.Store.InIdentityTx(context.Background(), func(store repository.IdentityStore) error {
		count, err := store.User().QueryResisterUserTotal(context.Background())
		if err != nil {
			return err
		}
		if count == 0 {
			email := identifier.CanonicalEmail(current.Administrator.Email)
			if email == "" {
				return errors.New("administrator email is required")
			}
			encoded := current.Administrator.PasswordHash
			if encoded != "" {
				if password.PasswordNeedsRehash(password.PasswordAlgoArgon2id, encoded) {
					return errors.New("invalid staged administrator password hash")
				}
			} else {
				if current.Administrator.Password == "" {
					return errors.New("administrator password is required")
				}
				encoded = password.EncodePassWord(current.Administrator.Password)
			}
			enable := true
			admin := &user.User{
				Password:  encoded,
				Algo:      password.PasswordAlgoArgon2id,
				IsAdmin:   &enable,
				ReferCode: user.GenerateInviteCode(time.Now().Unix()),
			}
			if err := store.User().Insert(context.Background(), admin); err != nil {
				logger.Errorf("[Migrate] CreateAdminUser error: %v", err.Error())
				return err
			}
			if err := store.UserAuth().InsertUserAuthMethods(context.Background(), &user.AuthMethods{
				UserId:         admin.Id,
				AuthType:       "email",
				AuthIdentifier: email,
				Verified:       true,
			}); err != nil {
				logger.Errorf("[Migrate] CreateAdminUser error: %v", err.Error())
				return err
			}
			logger.Info("[Migrate] Create admin user success")
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	// Recovery is complete only after the durable seed and atomic staged
	// secret cleanup. Failure stops startup and remains safe to retry.
	if current.Administrator.PasswordHash != "" {
		if ctx.FinalizeStagedAdmin == nil {
			panic("staged administrator cleanup is unavailable")
		}
		if err := ctx.FinalizeStagedAdmin(current.Administrator.PasswordHash); err != nil {
			panic(err)
		}
		ctx.updateConfig(func(c *config.Config) { c.Administrator.PasswordHash = "" })
	}
}
