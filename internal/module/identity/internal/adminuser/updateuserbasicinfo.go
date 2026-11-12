package adminuser

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/perfect-panel/server/internal/auth/password"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateUserBasicInfoLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewUpdateUserBasicInfoLogic Update user basic info
func newUpdateUserBasicInfoLogic(ctx context.Context, deps Deps) *UpdateUserBasicInfoLogic {
	return &UpdateUserBasicInfoLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateUserBasicInfoLogic) UpdateUserBasicInfo(req *dto.UpdateUserBasiceInfoRequest) error {
	isDemo := strings.ToLower(os.Getenv("PPANEL_MODE")) == "demo"
	// The admin edit spans two domains by design — identity profile fields
	// and a billing money adjustment — so it runs as two sequential domain
	// transactions. The identity transaction goes first because it carries
	// the request validations (avatar, demo-mode password): a rejected edit
	// then leaves the money untouched. A failure after the profile commit
	// leaves the money unadjusted for the admin to retry — the same
	// partial-failure surface the flows will have as services.
	accessStateChanged := false
	err := l.deps.Store.InIdentityTx(l.ctx, func(store repository.IdentityStore) error {
		userInfo, err := store.User().FindOneForUpdate(l.ctx, req.UserId)
		if err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Find User Error")
		}
		if err := validateAvatarUpdate(userInfo.Avatar, req.Avatar); err != nil {
			return err
		}
		userInfo.Avatar = req.Avatar
		userInfo.ReferCode = req.ReferCode
		userInfo.RefererId = req.RefererId
		userInfo.OnlyFirstPurchase = &req.OnlyFirstPurchase
		userInfo.ReferralPercentage = req.ReferralPercentage
		// Checked on the locked row so two edits cannot each count the other.
		if isEnabledAdministrator(userInfo) && (!req.IsAdmin || !req.Enable) {
			if err := ensureAnotherAdministrator(l.ctx, store.User(), userInfo.Id); err != nil {
				return err
			}
		}
		accessStateChanged = userInfo.Enable == nil || *userInfo.Enable != req.Enable
		userInfo.Enable = &req.Enable
		userInfo.IsAdmin = &req.IsAdmin
		if req.Password != "" && req.Password != "***" {
			if userInfo.Id == 2 && isDemo {
				return errors.Wrapf(xerr.NewErrCodeMsg(503, "Demo mode does not allow modification of the admin user password"), "UpdateUserBasicInfo failed: cannot update admin user password in demo mode")
			}
			userInfo.Password = password.EncodePassWord(req.Password)
			userInfo.Algo = password.PasswordAlgoArgon2id
			userInfo.Salt = ""
		}
		// The profile save skips the billing-owned money columns; the
		// admin's wallet adjustment runs in its own billing transaction
		// below.
		return store.User().Update(l.ctx, userInfo)
	})
	if err != nil {
		l.Errorw("[UpdateUserBasicInfoLogic] Update User Error:", logger.Field("err", err.Error()), logger.Field("userId", req.UserId))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "Update User Error")
	}
	// Account state changes must invalidate both subscription-token caches and
	// node-facing user lists. In particular, disabling a user takes effect at
	// the service plane immediately instead of waiting for the five-minute TTL.
	if accessStateChanged {
		clearUserAccessCaches(l.ctx, l.deps, []int64{req.UserId})
	}

	err = l.deps.Store.InBillingTx(l.ctx, func(store repository.BillingStore) error {
		// Financial adjustments must compare and write the latest values
		// under the wallet lock, with their audit logs in the same
		// transaction.
		walletInfo, err := store.Wallet().FindOneForUpdate(l.ctx, req.UserId)
		if err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Find User Wallet Error")
		}
		if walletInfo.Balance == req.Balance &&
			walletInfo.GiftAmount == req.GiftAmount &&
			walletInfo.Commission == req.Commission {
			return nil
		}
		if walletInfo.Balance != req.Balance {
			content, _ := (&log.Balance{Type: log.BalanceTypeAdjust, Amount: req.Balance - walletInfo.Balance, Balance: req.Balance, Timestamp: timeutil.Now().UnixMilli()}).Marshal()
			if err := store.Log().Insert(l.ctx, &log.SystemLog{Type: log.TypeBalance.Uint8(), Date: timeutil.Now().Format(time.DateOnly), ObjectID: req.UserId, Content: string(content)}); err != nil {
				return err
			}
		}
		if walletInfo.GiftAmount != req.GiftAmount {
			changeType := log.GiftTypeReduce
			if req.GiftAmount > walletInfo.GiftAmount {
				changeType = log.GiftTypeIncrease
			}
			content, _ := (&log.Gift{Type: changeType, Amount: req.GiftAmount - walletInfo.GiftAmount, Balance: req.GiftAmount, Remark: "Admin adjustment", Timestamp: timeutil.Now().UnixMilli()}).Marshal()
			if err := store.Log().Insert(l.ctx, &log.SystemLog{Type: log.TypeGift.Uint8(), Date: timeutil.Now().Format(time.DateOnly), ObjectID: req.UserId, Content: string(content)}); err != nil {
				return err
			}
		}
		if walletInfo.Commission != req.Commission {
			content, _ := (&log.Commission{Type: log.CommissionTypeAdjust, Amount: req.Commission - walletInfo.Commission, Timestamp: timeutil.Now().UnixMilli()}).Marshal()
			if err := store.Log().Insert(l.ctx, &log.SystemLog{Type: log.TypeCommission.Uint8(), Date: timeutil.Now().Format(time.DateOnly), ObjectID: req.UserId, Content: string(content)}); err != nil {
				return err
			}
		}
		walletInfo.Balance = req.Balance
		walletInfo.GiftAmount = req.GiftAmount
		walletInfo.Commission = req.Commission
		if err := store.Wallet().UpdateBalanceFields(l.ctx, walletInfo); err != nil {
			return err
		}
		return store.Wallet().UpdateCommission(l.ctx, walletInfo)
	})
	if err != nil {
		l.Errorw("[UpdateUserBasicInfoLogic] Adjust User Wallet Error:", logger.Field("err", err.Error()), logger.Field("userId", req.UserId))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "Update User Error")
	}

	return nil
}

// validateAvatarUpdate permits retaining or clearing an existing avatar. A new
// avatar must be a Base64 image no larger than 1024 KiB; OAuth providers may
// persist remote HTTPS avatar URLs, which must remain usable during unrelated
// profile updates.
func validateAvatarUpdate(currentAvatar, requestedAvatar string) error {
	if requestedAvatar == "" || requestedAvatar == currentAvatar {
		return nil
	}

	if !IsValidImageSize(requestedAvatar, 1024) {
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "Invalid avatar")
	}

	return nil
}
