// Package sched generates crontab lines for scheduled hlops runs, including
// Termux wake-lock wrappers and human-readable installation instructions.
package sched

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eddievibe4tech-byte/termux-ops/internal/paths"
	"github.com/eddievibe4tech-byte/termux-ops/internal/termux"
)

// CronOptions configures generation.
type CronOptions struct {
	Job      string // backup | cleanup
	Time     string // 5-field cron expression, e.g. "0 2 * * *"
	Binary   string // path to hlops; default: resolved from PATH or $PREFIX/bin
	WakeLock bool   // wrap with termux-wake-lock/unlock (default true on Termux)
}

// ValidateTime checks a 5-field cron expression.
func ValidateTime(expr string) error {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Errorf("cron expression must have 5 fields, got %d (%q)", len(fields), expr)
	}
	for _, f := range fields {
		if f == "" {
			return fmt.Errorf("empty cron field in %q", expr)
		}
		for _, r := range f {
			if !(r >= '0' && r <= '9' || r == '*' || r == ',' || r == '-' || r == '/') {
				return fmt.Errorf("invalid character %q in cron field %q", string(r), f)
			}
		}
	}
	return nil
}

// GenerateCron returns the cron line plus installation instructions.
func GenerateCron(o CronOptions) (string, error) {
	if o.Job != "backup" && o.Job != "cleanup" {
		return "", fmt.Errorf("job must be backup or cleanup, got %q", o.Job)
	}
	if o.Time == "" {
		o.Time = "0 3 * * *"
	}
	if err := ValidateTime(o.Time); err != nil {
		return "", err
	}
	bin := o.Binary
	if bin == "" {
		bin = detectBinary()
	}
	logFile := filepath.Join(paths.LogDir(), "cron.log")

	info := termux.Detect()
	wrap := o.WakeLock || info.IsTermux

	var body string
	if wrap {
		body = fmt.Sprintf("termux-wake-lock || true; %s run %s >> %s 2>&1; rc=$?; termux-wake-unlock || true; exit $rc",
			bin, o.Job, logFile)
	} else {
		body = fmt.Sprintf("%s run %s >> %s 2>&1", bin, o.Job, logFile)
	}

	line := fmt.Sprintf("%s %s", o.Time, body)

	var out strings.Builder
	out.WriteString("# hlops cron entry\n")
	out.WriteString(line + "\n")
	out.WriteString("\n# --- how to install ---\n")
	out.WriteString("# 1. Install cronie (Termux):\n")
	out.WriteString("#    pkg install cronie termux-services\n")
	out.WriteString("# 2. Start crond now:\n")
	out.WriteString("#    sv-enable crond        # via termux-services\n")
	out.WriteString("#    (or run `crond` manually for a one-off test)\n")
	out.WriteString("# 3. Add the line above to your crontab:\n")
	out.WriteString("#    crontab -e             # then paste the line\n")
	out.WriteString("#    (or: hlops schedule generate cron --time \"0 2 * * *\" | tail -1 | tee -a ~/.hlops-cron && crontab ~/.hlops-cron)\n")
	out.WriteString("# 4. Battery optimisation: exclude Termux from Android battery\n")
	out.WriteString("#    optimisation (Settings > Apps > Termux > Battery > Unrestricted),\n")
	out.WriteString("#    otherwise Doze may delay or drop cron jobs.\n")
	if info.IsTermux {
		out.WriteString("# Note: this entry uses termux-wake-lock so the job survives\n")
		out.WriteString("# screen-off; wake-unlock runs after the job finishes.\n")
	}
	return out.String(), nil
}

func detectBinary() string {
	if p, err := os.Executable(); err == nil {
		return p
	}
	info := termux.Detect()
	if info.IsTermux && info.Prefix != "" {
		return filepath.Join(info.Prefix, "bin", "hlops")
	}
	return "hlops"
}
