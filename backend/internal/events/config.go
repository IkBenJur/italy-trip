package events

import (
	"time"

	"github.com/IkBenJur/italy-trip/internal/env"
)

// Config is the event as described by the environment. It is read once at boot
// and upserted onto the singleton event row, which makes the unlock moment a
// Railway variable rather than something that needs manual SQL.
type Config struct {
	Name     string
	StartsAt time.Time
	EndsAt   time.Time
}

// ConfigFromEnv reads EVENT_NAME, EVENT_STARTS_AT and EVENT_ENDS_AT, aborting
// the process if any is missing or malformed.
func ConfigFromEnv() Config {
	return Config{
		Name:     env.MustGet("EVENT_NAME"),
		StartsAt: env.MustTime("EVENT_STARTS_AT"),
		EndsAt:   env.MustTime("EVENT_ENDS_AT"),
	}
}
