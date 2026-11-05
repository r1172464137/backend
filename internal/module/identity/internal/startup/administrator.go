package startup

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/account"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
)

// CreateInitialAdministrator creates the first administrator, signing in
// with the verified email address (stored in its canonical form, the one
// sign-in looks up) and the password plain, when the database holds no
// account yet, and reports whether it created one. It is idempotent and safe
// to run on every start: once any account exists, the seeded administrator
// among them, nothing is created, so a configured email another account
// holds cannot fail a later start either. The count and the account share
// one identity transaction, so the account and its email binding are
// created together or not at all, and two starts cannot both seed. The refer
// code is set up front, as the bootstrap always did, so the account is
// written without a follow-up update.
func (s *Service) CreateInitialAdministrator(ctx context.Context, email, plain string) (bool, error) {
	return s.createInitialAdministrator(ctx, email, func() (string, error) { return password.EncodePassWord(plain), nil })
}

// CreateInitialAdministratorFromHash restores the exact staged Argon2id hash.
func (s *Service) CreateInitialAdministratorFromHash(ctx context.Context, email, hash string) (bool, error) {
	return s.createInitialAdministrator(ctx, email, func() (string, error) {
		if !validArgon2id(hash) {
			return "", xerr.Wrapf(fmt.Errorf("invalid Argon2id PHC"), xerr.InvalidParams, "the administrator password hash is not valid")
		}
		return hash, nil
	})
}

func validArgon2id(hash string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	seen := map[string]bool{}
	for _, item := range strings.Split(parts[3], ",") {
		k, v, ok := strings.Cut(item, "=")
		if !ok || seen[k] {
			return false
		}
		seen[k] = true
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil || n == 0 {
			return false
		}
		switch k {
		case "m":
			if n > 256*1024 {
				return false
			}
		case "t":
			if n > 20 {
				return false
			}
		case "p":
			if n > 8 {
				return false
			}
		default:
			return false
		}
	}
	if len(seen) != 3 {
		return false
	}
	for _, v := range parts[4:] {
		b, err := base64.RawStdEncoding.DecodeString(v)
		if err != nil || len(b) == 0 {
			return false
		}
	}
	return true
}

func (s *Service) createInitialAdministrator(ctx context.Context, email string, getHash func() (string, error)) (bool, error) {
	created := false
	err := s.deps.Store.InIdentityTx(ctx, func(tx repository.IdentityStore) error {
		count, err := tx.User().QueryRegisterUserTotal(ctx)
		if err != nil {
			return xerr.Wrapf(err, xerr.DatabaseQueryError, "count the accounts")
		}
		if count > 0 {
			return nil
		}
		canonicalEmail, err := identifier.ValidateEmail(email, "", false)
		if err != nil {
			return xerr.Wrapf(err, xerr.InvalidParams, "the administrator email is not valid")
		}
		hash, err := getHash()
		if err != nil {
			return err
		}
		isAdmin := true
		if err := account.Create(ctx, tx, account.New{
			User: &user.User{
				Password:  hash,
				Algo:      password.PasswordAlgoArgon2id,
				IsAdmin:   &isAdmin,
				ReferCode: user.GenerateInviteCode(timeutil.Now().Unix()),
			},
			Identities: []user.AuthMethods{{AuthType: identifier.Email, AuthIdentifier: canonicalEmail, Verified: true}},
		}); err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return created, nil
}

// FindAdministratorsWithPassword returns the administrators, with their auth
// methods, who sign in with the password plain: the startup warns about those
// still using the password older releases seeded, which anyone can look up.
func (s *Service) FindAdministratorsWithPassword(ctx context.Context, plain string) ([]*user.User, error) {
	admins, err := s.deps.Users.QueryAdminUsers(ctx)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "query the administrators")
	}
	var matching []*user.User
	for _, admin := range admins {
		if password.MultiPasswordVerify(admin.Algo, admin.Salt, plain, admin.Password) {
			matching = append(matching, admin)
		}
	}
	return matching, nil
}
