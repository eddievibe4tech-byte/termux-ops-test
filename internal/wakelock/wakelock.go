// Package wakelock integrates Termux wake-lock around long-running jobs.
// On non-Termux systems everything is a no-op; when termux-api binaries are
// missing we only warn and never abort the job.
package wakelock

import (
	"log/slog"
	"os/exec"

	"github.com/eddievibe4tech-byte/termux-ops/internal/termux"
)

// Guard holds a wake lock until Release.
type Guard struct {
	active bool
}

// Acquire runs termux-wake-lock when on Termux and the tool exists.
// It never returns an error: failures degrade to warnings.
func Acquire() *Guard {
	info := termux.Detect()
	if !info.IsTermux {
		slog.Debug("wakelock skipped: not termux")
		return &Guard{}
	}
	if _, err := exec.LookPath("termux-wake-lock"); err != nil {
		slog.Warn("termux-api not installed; skipping wake-lock (install with: pkg install termux-api)")
		return &Guard{}
	}
	if err := exec.Command("termux-wake-lock").Run(); err != nil {
		slog.Warn("termux-wake-lock failed", "err", err)
		return &Guard{}
	}
	slog.Info("termux wake-lock acquired")
	return &Guard{active: true}
}

// Release runs termux-wake-unlock if we hold the lock.
func (g *Guard) Release() {
	if g == nil || !g.active {
		return
	}
	if err := exec.Command("termux-wake-unlock").Run(); err != nil {
		slog.Warn("termux-wake-unlock failed", "err", err)
	} else {
		slog.Info("termux wake-lock released")
	}
	g.active = false
}
