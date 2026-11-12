package delivery

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/repository"
)

// Config is the runtime configuration snapshot for a delivery request; it is
// re-read per request because the subscribe/site settings are mutable.
type Config struct {
	SiteName              string
	Host                  string
	SubscribeDomain       string
	ProfileUpdateInterval int64
	ProfileWebPageURL     string
	UserAgentList         string
}

type Deps struct {
	Clients  repository.ClientRepo
	Plans    repository.SubscribeRepo
	UserSubs repository.UserSubscriptionRepo
	// Users is the identity read port for the account-enabled gate.
	Users repository.UserRepo
	Nodes repository.NodeRepo
	Logs  repository.LogRepo
	// Limiter bounds fetches per client address; nil admits every fetch.
	Limiter FetchLimiter
	// ConfigSnapshot reads the current delivery configuration.
	ConfigSnapshot func() Config
}

func (d Deps) config() Config {
	if d.ConfigSnapshot == nil {
		return Config{}
	}
	return d.ConfigSnapshot()
}

type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

// Deliver renders the client configuration for a subscription token.
func (s *Service) Deliver(ctx context.Context, meta RequestMeta, req *dto.SubscribeRequest) (*dto.SubscribeResponse, error) {
	return newSubscribeLogic(ctx, s.deps, meta).Handler(req)
}
