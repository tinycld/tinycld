//go:build classifierfake

package fakeprovider

import "tinycld.org/core/classifier"

// Built into dev and e2e servers only (scripts/dev.ts, scripts/e2e-serve.ts
// pass -tags classifierfake), so they never call OpenRouter.
func init() {
	classifier.SetDefaultProvider(func() classifier.Provider { return New() })
}
