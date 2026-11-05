package xerr

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

var errStore = errors.New("store unavailable")

// A generic outer code must not hide the specific code an inner layer chose:
// an exhausted coupon inside a transaction reaches the client as such, not as
// an internal error.
func TestWrapfKeepsInnerCodeUnderGenericCode(t *testing.T) {
	inner := Errorf(CouponInsufficientUsage, "coupon used or expired")
	for _, code := range []uint32{ERROR, DatabaseQueryError, DatabaseUpdateError, DatabaseInsertError, DatabaseDeletedError} {
		err := Wrapf(inner, code, "transaction error: %v", inner.Error())
		if got := CodeOf(err); got != CouponInsufficientUsage {
			t.Fatalf("code %d replaced the inner code: got %d", code, got)
		}
		if !strings.HasPrefix(err.Error(), "transaction error: ") {
			t.Fatalf("context message lost: %q", err.Error())
		}
	}
}

// A specific outer code is a deliberate translation and wins.
func TestWrapfSpecificCodeReplacesInnerCode(t *testing.T) {
	inner := NewErrCode(InvalidParams)
	if got := CodeOf(Wrapf(inner, UserNotExist, "lookup")); got != UserNotExist {
		t.Fatalf("code = %d, want %d", got, UserNotExist)
	}
}

// An uncoded cause gets the code and stays reachable, so callers further up
// can still test for it.
func TestWrapfAttachesCodeAndKeepsCause(t *testing.T) {
	err := Wrapf(errStore, DatabaseQueryError, "find user %d: %v", 7, errStore.Error())
	if got := CodeOf(err); got != DatabaseQueryError {
		t.Fatalf("code = %d, want %d", got, DatabaseQueryError)
	}
	if !errors.Is(err, errStore) {
		t.Fatal("cause is no longer reachable through errors.Is")
	}
	want := fmt.Sprintf("find user 7: %s: %s", errStore, NewErrCode(DatabaseQueryError))
	if err.Error() != want {
		t.Fatalf("message = %q, want %q (the context, the cause, then the code)", err.Error(), want)
	}
	// The response layer finds the code by walking the chain.
	var coded *CodeError
	if !errors.As(err, &coded) || coded.GetErrCode() != DatabaseQueryError {
		t.Fatalf("code not found in the chain of %v", err)
	}
}

func TestWrapfNil(t *testing.T) {
	if err := Wrapf(nil, ERROR, "nothing"); err != nil {
		t.Fatalf("Wrapf(nil) = %v, want nil", err)
	}
}

func TestCodeOf(t *testing.T) {
	if got := CodeOf(errStore); got != ERROR {
		t.Fatalf("CodeOf(uncoded) = %d, want ERROR", got)
	}
	if got := CodeOf(fmt.Errorf("register: %w", NewErrCode(UserExist))); got != UserExist {
		t.Fatalf("CodeOf through a wrap = %d, want %d", got, UserExist)
	}
}

func TestDetailIncludesCauseOnce(t *testing.T) {
	quiet := Wrapf(errStore, DatabaseQueryError, "find order")
	if got := Detail(quiet); !strings.Contains(got, errStore.Error()) {
		t.Fatalf("Detail(%q) = %q, want the cause included", quiet, got)
	}
	loud := Wrapf(errStore, DatabaseQueryError, "find order: %v", errStore)
	if got := Detail(loud); strings.Count(got, errStore.Error()) != 1 {
		t.Fatalf("Detail(%q) = %q, want the cause exactly once", loud, got)
	}
	if Detail(nil) != "" {
		t.Fatal("Detail(nil) is not empty")
	}
}

func TestErrorfCarriesCode(t *testing.T) {
	err := Errorf(InvalidParams, "invalid email: %s", "x")
	if got := CodeOf(err); got != InvalidParams {
		t.Fatalf("code = %d, want %d", got, InvalidParams)
	}
	want := "invalid email: x: " + NewErrCode(InvalidParams).Error()
	if err.Error() != want {
		t.Fatalf("message = %q, want %q (the context, then the code)", err.Error(), want)
	}
}
