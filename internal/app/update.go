package app

import (
	"context"
	"time"

	"github.com/scuq/f9/internal/updater"
)

// updateRepo is the GitHub repo checked for new releases.
const updateRepo = "scuq/f9"

// CheckForUpdate reports whether a newer release is available. Safe to call on
// startup; errors are returned in the Info, not raised.
func (a *App) CheckForUpdate() updater.Info {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	return updater.Check(ctx, updateRepo, Version)
}
