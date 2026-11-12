package handler

import (
	"errors"
	"testing"

	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/perfect-panel/server/pkg/xerr"
	pkgerrors "github.com/pkg/errors"
)

// CONTRACT 5.3: every refusal is a plain-text status, never a config body.
func TestSubscribeErrorStatus(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{pkgerrors.Wrap(xerr.NewErrCode(xerr.ErrorTokenInvalid), "x"), consts.StatusNotFound},
		{pkgerrors.Wrap(xerr.NewErrCode(xerr.UserDisabled), "x"), consts.StatusForbidden},
		{pkgerrors.Wrap(xerr.NewErrCode(xerr.UserNotExist), "x"), consts.StatusForbidden},
		{pkgerrors.Wrap(xerr.NewErrCode(xerr.TooManyRequests), "x"), consts.StatusTooManyRequests},
		{pkgerrors.Wrap(xerr.NewErrCodeMsg(xerr.InvalidParams, "unknown subscribe application"), "x"), consts.StatusBadRequest},
		{pkgerrors.Wrap(xerr.NewErrCode(500), "render"), consts.StatusInternalServerError},
		{errors.New("plain"), consts.StatusInternalServerError},
	}
	for _, c := range cases {
		if got, msg := subscribeErrorStatus(c.err); got != c.want || msg == "" {
			t.Errorf("%v: %d %q, want %d", c.err, got, msg, c.want)
		}
	}
}
