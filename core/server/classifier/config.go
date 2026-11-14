package classifier

import (
	"errors"
	"math"
	"strconv"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/syscfg"
)

const (
	KeyAPIKey        = "classifier.api_key"
	KeyBudget        = "classifier.monthly_budget_usd"
	KeyFullThreshold = "classifier.full_evidence_threshold"
	KeyStatus        = "classifier.status"

	KeyAutoActionThreshold = "classifier.auto_action_threshold"

	defaultBudget              = 5.0
	defaultFullThreshold       = 0.7
	defaultAutoActionThreshold = 0.9
	minAutoActionThreshold     = 0.5
	StatusOK                   = "ok"
)

var (
	ErrUnavailable    = errors.New("classifier: not configured or paused")
	ErrBudgetExceeded = errors.New("classifier: monthly budget exceeded")
)

type Config struct {
	APIKey        string
	Status        string
	Budget        float64
	FullThreshold float64
	// AutoActionThreshold is the confidence an app needs before it acts on a
	// result by itself (for example archiving).
	AutoActionThreshold float64
}

func floatSetting(app core.App, key string, fallback float64) float64 {
	v, err := strconv.ParseFloat(syscfg.Get(app, key), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return fallback
	}
	return v
}

func LoadConfig(app core.App) Config {
	cfg := Config{
		APIKey:        syscfg.Get(app, KeyAPIKey),
		Status:        syscfg.Get(app, KeyStatus),
		Budget:        floatSetting(app, KeyBudget, defaultBudget),
		FullThreshold: floatSetting(app, KeyFullThreshold, defaultFullThreshold),
	}
	cfg.AutoActionThreshold = autoActionThreshold(app)
	if cfg.Status == "" {
		cfg.Status = StatusOK
	}
	return cfg
}

// autoActionThreshold falls back to the default for an out-of-range value
// instead of clamping: below 0.5 the model's pick is little better than a
// coin flip, so a bad entry must not lower the bar for automatic actions.
func autoActionThreshold(app core.App) float64 {
	v, err := strconv.ParseFloat(syscfg.Get(app, KeyAutoActionThreshold), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < minAutoActionThreshold || v > 1 {
		return defaultAutoActionThreshold
	}
	return v
}

// Available reports whether classification may run: an admin has entered a
// key and no call has since failed in a way every later call would repeat.
func Available(app core.App) bool {
	cfg := LoadConfig(app)
	return cfg.APIKey != "" && cfg.Status == StatusOK
}

// setStatus writes classifier.status through the system_settings row, so the
// settings cache and the admin panel both see it.
func setStatus(app core.App, status string) error {
	rec, err := app.FindFirstRecordByFilter("system_settings", "key = {:k}", map[string]any{"k": KeyStatus})
	if err != nil {
		col, cerr := app.FindCollectionByNameOrId("system_settings")
		if cerr != nil {
			return cerr
		}
		rec = core.NewRecord(col)
		rec.Set("key", KeyStatus)
		rec.Set("is_secret", false)
	}
	if rec.GetString("value") == status {
		return nil
	}
	rec.Set("value", status)
	return app.Save(rec)
}
