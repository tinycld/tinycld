package coreserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"

	gopbs "github.com/osshield/gopbs/pbs"

	"tinycld.org/core/backup/pbs"
)

// repositoryCallTimeout bounds a live call to the configured repository
// (listing snapshots, or testing settings before they are saved). Without a
// cap, an unreachable or hanging server would leave the request — and the
// caller's browser tab — waiting indefinitely.
const repositoryCallTimeout = 20 * time.Second

func handleSnapshots(app core.App, re *core.RequestEvent) error {
	r, err := openRepository(app)
	if err != nil {
		return re.BadRequestError(err.Error(), nil)
	}
	ctx, cancel := context.WithTimeout(re.Request.Context(), repositoryCallTimeout)
	defer cancel()
	list, err := r.List(ctx)
	if err != nil {
		return re.JSON(http.StatusBadGateway, map[string]string{"message": repositoryError(err)})
	}
	return re.JSON(http.StatusOK, list)
}

type repositoryTestBody struct {
	Kind   string          `json:"kind"`
	Config json.RawMessage `json:"config"`
}

// handleRepositoryTest tries settings before they are saved. It lists the
// snapshots and writes nothing.
func handleRepositoryTest(app core.App, re *core.RequestEvent) error {
	var body repositoryTestBody
	if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
		return re.BadRequestError("invalid JSON body", err)
	}
	r, err := openRepositoryWith(app, body.Kind, body.Config)
	if err != nil {
		return re.JSON(http.StatusUnprocessableEntity, map[string]string{"message": err.Error()})
	}
	ctx, cancel := context.WithTimeout(re.Request.Context(), repositoryCallTimeout)
	defer cancel()
	list, err := r.List(ctx)
	if err != nil {
		return re.JSON(http.StatusUnprocessableEntity, map[string]string{"message": repositoryError(err)})
	}
	return re.JSON(http.StatusOK, map[string]int{"snapshots": len(list)})
}

func handleGenerateKey(re *core.RequestEvent) error {
	var body struct {
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil || body.Kind != pbs.Kind {
		return re.BadRequestError("Only PBS keys can be generated.", err)
	}
	key, err := pbs.GenerateKey()
	if err != nil {
		return re.InternalServerError("could not generate a key", err)
	}
	// The key file is the only thing that can decrypt a PBS-encrypted backup,
	// so it must never be cached anywhere between here and the admin who copies
	// it down.
	re.Response.Header().Set("Cache-Control", "no-store")
	return re.JSON(http.StatusOK, map[string]string{"key": key})
}

// repositoryError maps gopbs's sentinel errors onto messages safe to return to
// a caller. The raw error is never used here — gopbs errors may name the
// server's host or a datastore path, and openRepositoryWith's own errors
// (config validation) are already safe and returned by their callers directly.
func repositoryError(err error) string {
	switch {
	case errors.Is(err, gopbs.ErrAuth):
		return "The repository refused the credentials."
	case errors.Is(err, gopbs.ErrFingerprint):
		return "The server's certificate does not match the fingerprint."
	case errors.Is(err, gopbs.ErrNotFound):
		return "The datastore or namespace does not exist."
	case errors.Is(err, context.DeadlineExceeded):
		return "The repository did not answer in time."
	default:
		return "Could not reach the repository."
	}
}
