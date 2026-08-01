package tenantcfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Deploy outcome statuses. Committed means the proposed build passed its
// readiness handshake and the org row was committed to it; Reverted means it
// failed and the router restored the pre-deploy snapshot and respawned the
// previous build.
const (
	DeployCommitted = "committed"
	DeployReverted  = "reverted"
)

// DeployResult is .runtime/deploy-result.json — the router's durable record
// of an org's most recent deploy outcome (design D6 step 5), the hosted
// analog of the pkg_install_log row that survives exit-75. The router writes
// it after the post-deploy respawn settles; the (re)spawned tenant reads it
// at boot to finalize the install-log row the Packages UI is polling by
// JobID, then removes it (consume-after-success, like the single-tenant
// .rollback-pending breadcrumb).
type DeployResult struct {
	// JobID echoes the tenant's proposal, tying the outcome back to the
	// pkg_install_log row the UI polls across the restart discontinuity.
	JobID string `json:"jobId,omitempty"`

	// Status is DeployCommitted or DeployReverted.
	Status string `json:"status"`

	// Error is the failure reason for a reverted deploy — the readiness
	// pipe's step-attributed cause, or the build/peer refusal.
	Error string `json:"error,omitempty"`

	// RecipeHash is the build the org is now running (the proposed build when
	// committed, the restored previous build when reverted).
	RecipeHash string `json:"recipeHash,omitempty"`

	CompletedAt time.Time `json:"completedAt"`
}

// DeployResultPath is where an org's deploy result lives. One definition for
// the router's writer and the tenant's loader.
func DeployResultPath(orgDir string) string {
	return filepath.Join(orgDir, ".runtime", "deploy-result.json")
}

// LoadDeployResult reads a deploy result. ok is false when none exists —
// unlike the boot configs, "no result" and "zero result" must be
// distinguishable, since the tenant consumes a result exactly once.
func LoadDeployResult(orgDir string) (res DeployResult, ok bool, err error) {
	body, err := os.ReadFile(DeployResultPath(orgDir))
	if err != nil {
		if os.IsNotExist(err) {
			return DeployResult{}, false, nil
		}
		return DeployResult{}, false, err
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return DeployResult{}, false, err
	}
	return res, true, nil
}

// ConsumeDeployResult removes the result after the tenant has durably
// recorded it. Removing an absent file is a no-op.
func ConsumeDeployResult(orgDir string) error {
	err := os.Remove(DeployResultPath(orgDir))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
