package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/mail"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/internal/transport/devicesocket"
	"github.com/perfect-panel/server/pkg/logger"
)

// newSubscriptionModule wires the subscription module against the legacy
// store; device broadcast and the runtime-mutable trial plan are closures
// over the service context.
func newSubscriptionModule(store repository.Store, srv *Application) subscription.Service {
	return subscription.New(subscription.Deps{
		Plans:    store.Subscribe(),
		UserSubs: store.UserSubscription(),
		Nodes:    store.Node(),
		Store:    store,
		NotifyPlanChanged: func() {
			if srv.DeviceManager != nil {
				srv.DeviceManager.Broadcast(devicesocket.SubscribeUpdate)
			}
		},
		Host: srv.Runtime.Config().Host,
		IsTrialPlan: func(planID int64) bool {
			current := srv.Runtime.Config().Register
			return current.EnableTrial && current.TrialSubscribe == planID
		},
		Clients:         store.Client(),
		DeliveryLimiter: subscription.NewDeliveryLimiter(srv.Redis),
		Users:           store.User(),
		Logs:            store.Log(),
		Devices:         store.UserDevice(),
		Cache:           store.UserCache(),
		Traffic:         store.TrafficLog(),
		Orders:          store.Order(),
		Inbox:           store.Inbox(),
		Operations:      store,
		SingleModel:     func() bool { return srv.Runtime.Config().Subscribe.SingleModel },
		TrialPolicy: func() subscription.TrialPolicy {
			c := srv.Runtime.Config().Register
			return subscription.TrialPolicy{
				Enabled:  c.EnableTrial,
				PlanID:   c.TrialSubscribe,
				Duration: c.TrialTime,
				TimeUnit: c.TrialTimeUnit,
			}
		},
		UserAuths:       store.UserAuth(),
		LifecycleNotify: lifecycleNotifier{srv: srv},
		DeliveryConfig: func() subscription.DeliveryConfig {
			current := srv.Runtime.Config()
			return subscription.DeliveryConfig{
				SiteName:              current.Site.SiteName,
				Host:                  current.Host,
				SubscribeDomain:       current.Subscribe.SubscribeDomain,
				ProfileUpdateInterval: current.Subscribe.ProfileUpdateInterval,
				ProfileWebPageURL:     current.Subscribe.ProfileWebPageURL,
				UserAgentList:         current.Subscribe.UserAgentList,
			}
		},
	})
}

// lifecycleNotifier adapts the subscription sweep's owner notices to their
// delivery channel: expiry and traffic notices go to the email queue, while
// the pre-expiry reminder goes over Telegram. Site branding is read per send
// because the admin can change it at runtime.
type lifecycleNotifier struct {
	srv *Application
}

func (n lifecycleNotifier) enqueue(ctx context.Context, payload taskqueue.SendEmailPayload, userEmail string) {
	body, err := json.Marshal(payload)
	if err != nil {
		logger.Errorw("[CheckSubscription] Marshal payload failed", logger.Field("error", err.Error()))
		return
	}
	task := asynq.NewTask(taskqueue.ForthwithSendEmail, body)
	info, err := n.srv.Queue.EnqueueContext(ctx, task, asynq.MaxRetry(3))
	if err != nil {
		logger.Errorw("[CheckSubscription] Enqueue task failed", logger.Field("error", err.Error()), logger.Field("payload", string(body)))
		return
	}
	logger.Infow("[CheckSubscription] Send email success",
		logger.Field("taskID", info.ID), logger.Field("Email", userEmail))
}

func (n lifecycleNotifier) NotifySubscriptionExpired(ctx context.Context, email string, expiredAt time.Time) {
	current := n.srv.Runtime.Config()
	n.enqueue(ctx, taskqueue.SendEmailPayload{
		Type:    taskqueue.EmailTypeExpiration,
		Email:   email,
		Subject: mail.DefaultExpirationEmailSubject,
		Content: map[string]interface{}{
			"SiteLogo":   current.Site.SiteLogo,
			"SiteName":   current.Site.SiteName,
			"ExpireDate": expiredAt.Format("2006-01-02 15:04:05"),
		},
	}, email)
}

func (n lifecycleNotifier) NotifyTrafficExceeded(ctx context.Context, email string) {
	current := n.srv.Runtime.Config()
	n.enqueue(ctx, taskqueue.SendEmailPayload{
		Type:    taskqueue.EmailTypeTrafficExceed,
		Email:   email,
		Subject: mail.DefaultTrafficExceedEmailSubject,
		Content: map[string]interface{}{
			"SiteLogo": current.Site.SiteLogo,
			"SiteName": current.Site.SiteName,
		},
	}, email)
}

// NotifySubscriptionExpiring warns the owner over Telegram before the
// subscription stops. Telegram is the only channel here: the email templates
// cover expiry after the fact, and the notice is gated on the operator's
// notification switch like every other bot message.
func (n lifecycleNotifier) NotifySubscriptionExpiring(ctx context.Context, userID int64, planName string, expireAt time.Time, renewalAmount int64) {
	if !n.srv.Runtime.Config().Telegram.EnableNotify {
		return
	}
	if planName == "" {
		planName = "订阅"
	}
	text, err := notification.RenderTelegramMarkdown(notification.SubscribeExpireNotify, map[string]string{
		"SubscribeName": planName,
		"ExpiredAt":     expireAt.Format("2006-01-02 15:04:05"),
		"RenewalAmount": fmt.Sprintf("%.2f", float64(renewalAmount)/100),
	})
	if err != nil {
		logger.Errorw("[RemindExpiring] Render template failed", logger.Field("error", err.Error()))
		return
	}
	if err := n.srv.Notification.NotifyTelegramUser(ctx, userID, text); err != nil {
		logger.Infow("[RemindExpiring] Telegram notice skipped",
			logger.Field("user_id", userID),
			logger.Field("reason", err.Error()),
		)
	}
}
