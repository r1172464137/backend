// Package systemsetting implements the system configuration subdomain of the
// platform module. Runtime subsystem re-initialization, restart and the
// mutable configuration snapshot are reached through injected callbacks.
package systemsetting

import (
	"context"
	"time"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/repository"
)

// MultiplierFunc evaluates the current node traffic multiplier.
type MultiplierFunc = func(at time.Time) float32

// PlatformTransactor mirrors the store's platform-scoped transaction.
type PlatformTransactor interface {
	InPlatformTx(ctx context.Context, fn func(repository.PlatformStore) error) error
}

type Deps struct {
	System       repository.SystemRepo
	Store        PlatformTransactor
	RoutingUsers repository.UserRepo
	// Reinitialize re-runs a subsystem's initialization after its
	// configuration changed.
	Reinitialize func(subsystem string)
	// Restart restarts the transport server (subscribe path changes).
	Restart func() error
	// SubscribePath reads the currently active subscribe path.
	SubscribePath func() string
	// ApplyVerifyConfig propagates the verify settings to the running
	// configuration before re-initialization.
	ApplyVerifyConfig func(req *dto.VerifyConfig)
	// Multiplier evaluates the current node traffic multiplier.
	Multiplier func(at time.Time) float32
}

func (d Deps) reinit(subsystem string) {
	if d.Reinitialize != nil {
		d.Reinitialize(subsystem)
	}
}

func (d Deps) restart() error {
	if d.Restart == nil {
		return nil
	}
	return d.Restart()
}

func (d Deps) subscribePath() string {
	if d.SubscribePath == nil {
		return ""
	}
	return d.SubscribePath()
}

func (d Deps) applyVerifyConfig(req *dto.VerifyConfig) {
	if d.ApplyVerifyConfig != nil {
		d.ApplyVerifyConfig(req)
	}
}

func (d Deps) multiplier(at time.Time) float32 {
	if d.Multiplier == nil {
		return 1
	}
	return d.Multiplier(at)
}

type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) GetCurrencyConfig(ctx context.Context) (*dto.CurrencyConfig, error) {
	return newGetCurrencyConfigLogic(ctx, s.deps).GetCurrencyConfig()
}

func (s *Service) GetInviteConfig(ctx context.Context) (*dto.InviteConfig, error) {
	return newGetInviteConfigLogic(ctx, s.deps).GetInviteConfig()
}

func (s *Service) GetNodeConfig(ctx context.Context) (*dto.NodeConfig, error) {
	return newGetNodeConfigLogic(ctx, s.deps).GetNodeConfig()
}

func (s *Service) GetNodeMultiplier(ctx context.Context) (*dto.GetNodeMultiplierResponse, error) {
	return newGetNodeMultiplierLogic(ctx, s.deps).GetNodeMultiplier()
}

func (s *Service) GetPrivacyPolicyConfig(ctx context.Context) (*dto.PrivacyPolicyConfig, error) {
	return newGetPrivacyPolicyConfigLogic(ctx, s.deps).GetPrivacyPolicyConfig()
}

func (s *Service) GetRegisterConfig(ctx context.Context) (*dto.RegisterConfig, error) {
	return newGetRegisterConfigLogic(ctx, s.deps).GetRegisterConfig()
}

func (s *Service) GetSiteConfig(ctx context.Context) (*dto.SiteConfig, error) {
	return newGetSiteConfigLogic(ctx, s.deps).GetSiteConfig()
}

func (s *Service) GetSubscribeConfig(ctx context.Context) (*dto.SubscribeConfig, error) {
	return newGetSubscribeConfigLogic(ctx, s.deps).GetSubscribeConfig()
}

func (s *Service) GetTosConfig(ctx context.Context) (*dto.TosConfig, error) {
	return newGetTosConfigLogic(ctx, s.deps).GetTosConfig()
}

func (s *Service) GetVerifyCodeConfig(ctx context.Context) (*dto.VerifyCodeConfig, error) {
	return newGetVerifyCodeConfigLogic(ctx, s.deps).GetVerifyCodeConfig()
}

func (s *Service) GetVerifyConfig(ctx context.Context) (*dto.VerifyConfig, error) {
	return newGetVerifyConfigLogic(ctx, s.deps).GetVerifyConfig()
}

func (s *Service) PreViewNodeMultiplier(ctx context.Context) (*dto.PreViewNodeMultiplierResponse, error) {
	return newPreViewNodeMultiplierLogic(ctx, s.deps).PreViewNodeMultiplier()
}

func (s *Service) SetNodeMultiplier(ctx context.Context, req *dto.SetNodeMultiplierRequest) error {
	return newSetNodeMultiplierLogic(ctx, s.deps).SetNodeMultiplier(req)
}

func (s *Service) SettingTelegramBot(ctx context.Context) error {
	return newSettingTelegramBotLogic(ctx, s.deps).SettingTelegramBot()
}

func (s *Service) UpdateCurrencyConfig(ctx context.Context, req *dto.CurrencyConfig) error {
	return newUpdateCurrencyConfigLogic(ctx, s.deps).UpdateCurrencyConfig(req)
}

func (s *Service) UpdateInviteConfig(ctx context.Context, req *dto.InviteConfig) error {
	return newUpdateInviteConfigLogic(ctx, s.deps).UpdateInviteConfig(req)
}

func (s *Service) UpdateNodeConfig(ctx context.Context, req *dto.NodeConfig) error {
	return newUpdateNodeConfigLogic(ctx, s.deps).UpdateNodeConfig(req)
}

func (s *Service) UpdatePrivacyPolicyConfig(ctx context.Context, req *dto.PrivacyPolicyConfig) error {
	return newUpdatePrivacyPolicyConfigLogic(ctx, s.deps).UpdatePrivacyPolicyConfig(req)
}

func (s *Service) UpdateRegisterConfig(ctx context.Context, req *dto.RegisterConfig) error {
	return newUpdateRegisterConfigLogic(ctx, s.deps).UpdateRegisterConfig(req)
}

func (s *Service) UpdateSiteConfig(ctx context.Context, req *dto.SiteConfig) error {
	return newUpdateSiteConfigLogic(ctx, s.deps).UpdateSiteConfig(req)
}

func (s *Service) UpdateSubscribeConfig(ctx context.Context, req *dto.SubscribeConfig) error {
	return newUpdateSubscribeConfigLogic(ctx, s.deps).UpdateSubscribeConfig(req)
}

func (s *Service) UpdateTosConfig(ctx context.Context, req *dto.TosConfig) error {
	return newUpdateTosConfigLogic(ctx, s.deps).UpdateTosConfig(req)
}

func (s *Service) UpdateVerifyCodeConfig(ctx context.Context, req *dto.VerifyCodeConfig) error {
	return newUpdateVerifyCodeConfigLogic(ctx, s.deps).UpdateVerifyCodeConfig(req)
}

func (s *Service) UpdateVerifyConfig(ctx context.Context, req *dto.VerifyConfig) error {
	return newUpdateVerifyConfigLogic(ctx, s.deps).UpdateVerifyConfig(req)
}
