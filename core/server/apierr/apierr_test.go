package apierr

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func newEvent() *core.RequestEvent {
	re := &core.RequestEvent{}
	re.Request = httptest.NewRequest(http.MethodPost, "/api/x", nil)
	re.Response = httptest.NewRecorder()
	return re
}

func asAPI(t *testing.T, err error) *router.ApiError {
	t.Helper()
	var got *router.ApiError
	if !errors.As(err, &got) {
		t.Fatalf("got %T %v, want *router.ApiError", err, err)
	}
	return got
}

func TestWrap_PassesAnApiErrorThrough(t *testing.T) {
	refusal := router.NewApiError(http.StatusPaymentRequired, "User limit reached: 3 of 3 seats in use.", nil)
	got := asAPI(t, Wrap(newEvent(), http.StatusBadRequest, "Failed to invite member", refusal))
	if got.Status != http.StatusPaymentRequired || got.Message != refusal.Message {
		t.Fatalf("got %d %q, want the 402 refusal unchanged", got.Status, got.Message)
	}
}

func TestWrap_PassesAWrappedApiErrorThrough(t *testing.T) {
	refusal := router.NewApiError(http.StatusPaymentRequired, "User limit reached", nil)
	got := asAPI(t, Wrap(newEvent(), http.StatusBadRequest, "x", fmt.Errorf("transaction: %w", refusal)))
	if got.Status != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402", got.Status)
	}
}

func TestWrap_UsesTheFallbackForOtherErrors(t *testing.T) {
	got := asAPI(t, Wrap(newEvent(), http.StatusInternalServerError, "Could not remove the user", errors.New("disk full")))
	if got.Status != http.StatusInternalServerError || got.Message != "Could not remove the user." {
		t.Fatalf("got %d %q, want a 500 with the fallback", got.Status, got.Message)
	}
}

func TestWrap_KeepsValidationFieldErrors(t *testing.T) {
	fields := validation.Errors{"username": validation.NewError("validation_not_unique", "Value must be unique.")}
	got := asAPI(t, Wrap(newEvent(), http.StatusBadRequest, "Failed to invite member", fields))
	if got.Data["username"] == nil {
		t.Fatalf("data = %v, want username", got.Data)
	}
}
