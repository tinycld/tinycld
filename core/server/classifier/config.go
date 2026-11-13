package classifier

import (
	"errors"
	"strconv"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/syscfg"
)

const (
	KeyAPIKey        = "classifier.api_key"
	KeyBudget        = "classifier.monthly_budget_usd"
	KeyFullThreshold = "classifier.full_evidence_threshold"
	KeyStatus        = "classifier.status"

	defaultBudget        = 5.0
	defaultFullThreshold = 0.7
	StatusOK             = "ok"
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
}

func floatSetting(app core.App, key string, fallback float64) float64 {
	v, err := strconv.ParseFloat(syscfg.Get(app, key), 64)
	if err != nil || v < 0 {
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
	if cfg.Status == "" {
		cfg.Status = StatusOK
	}
	return cfg
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
