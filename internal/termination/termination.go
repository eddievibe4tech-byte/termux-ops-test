// Package termination classifies how the hlops process ended, so scheduled
// runs can tell users whether a job "crashed" or was "forcibly terminated by
// the system" (issue #43, log/fault-tolerance layer).
//
// Background: on aggressive OEM ROMs (ColorOS / MIUI / OriginOS …) the
// framework's task killer sends SIGTERM/SIGKILL to Termux and everything it
// spawned, ignoring wake locks. A cron backup that "silently did not run"
// usually left no report behind at all — indistinguishable from a crash by
// naive tooling. This package provides:
//
//   - ClassifyExit: map an exit code + interrupted flag to a human
//     diagnosis string for logs.
//   - WatchSignals: install SIGINT/SIGTERM handlers that log a clear
//     "terminated by signal (possibly OEM task killer)" message before the
//     process exits with 128+signum, the conventional shell exit code for
//     signal deaths.
//   - DiagnoseMissingReport: when `hlops run` finished without writing its
//     JSON report, explain the most likely causes given the environment.
package termination

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// ExitSignalBase is the conventional exit code offset for death-by-signal
// (shell convention: 128 + signum). We mirror it so cron wrappers and the
// Companion APK can distinguish signal termination from plain errors (1).
const ExitSignalBase = 128

// WatchSignals installs SIGINT/SIGTERM handlers. On receipt it logs a
// structured warning naming the signal together with a full ClassifyExit
// diagnosis — printed *before* os.Exit, because os.Exit terminates the whole
// process immediately and no caller-side logging would ever run. It then
// exits with 128+signum after best-effort cleanup via onClose.
//
// getOEMAgg is called from the signal goroutine to fetch the current OEM
// aggressiveness flag; pass nil when the caller has no OEM context. There are
// deliberately no package-level "Interrupted"/"LastSignal" globals: writing
// them from this goroutine while other code reads them would be a data race,
// and since os.Exit fires here anyway, nothing downstream could observe them.
// Call once at process start (main). onClose may be nil.
func WatchSignals(onClose func(), getOEMAgg func() bool) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-ch
		sig, ok := s.(syscall.Signal)
		if !ok {
			sig = syscall.SIGTERM
		}
		code := ExitSignalBase + int(sig)
		var oemAgg bool
		if getOEMAgg != nil {
			oemAgg = getOEMAgg()
		}
		slog.Warn("hlops received termination signal",
			"signal", s.String(),
			"exit_code", code,
			"diagnosis", ClassifyExit(code, true, oemAgg),
			"hint", "if this was not you, an aggressive OEM battery manager "+
				"(ColorOS/MIUI/OriginOS) likely killed Termux despite the wake lock; "+
				"run 'hlops doctor' for the survival checklist")
		if onClose != nil {
			onClose()
		}
		os.Exit(code)
	}()
}

// ClassifyExit returns a one-line human diagnosis for a failed run, given the
// process exit code and whether a signal interrupted us. It never returns an
// empty string.
func ClassifyExit(code int, interrupted bool, oemAggressive bool) string {
	switch {
	case interrupted:
		// Derive the signal name from the exit code itself (143-128=15 →
		// SIGTERM) instead of relying on global state written by the signal
		// goroutine — that would be a data race and, post-os.Exit, unreadable.
		sigName := "unknown"
		if code > ExitSignalBase {
			sigName = syscall.Signal(code - ExitSignalBase).String()
		}
		msg := fmt.Sprintf("process was forcibly terminated (signal %s, exit %d)", sigName, code)
		if oemAggressive {
			return msg + ": known aggressive OEM power manager detected — see 'hlops doctor' checklist (dontkillmyapp.com)"
		}
		return msg + ": if unexpected, check OEM battery settings (see docs/termux-quickstart.md)"
	case code == ExitSignalBase+int(syscall.SIGKILL):
		// Note: SIGKILL (137) cannot be caught by this process — WatchSignals
		// never fires for it, so this classification is intended for external
		// wrappers or the Companion APK parsing the exit code after the fact.
		msg := "process was SIGKILLed (exit 137) — uncatchable; typical of low-memory kill (LMK) or OEM task killer"
		if oemAggressive {
			return msg + "; OEM ROM detected, follow 'hlops doctor' survival checklist"
		}
		return msg
	case code == 2:
		return "configuration syntax error (exit 2) — fix config.toml, not a crash"
	case code == 5 || code == 6:
		return fmt.Sprintf("another hlops instance holds the lock (exit %d) — skipped, not a crash", code)
	default:
		return fmt.Sprintf("job returned error (exit %d) — inspect logs under $HOME/.local/state/hlops/logs", code)
	}
}

// DiagnoseMissingReport explains why a scheduled `hlops run <job>` produced
// neither a completion report nor a bridge status document. The most common
// real-world cause on Android is the process being killed mid-run, which is
// exactly the confusion issue #43 describes ("排程有設，卻常常沒備份").
func DiagnoseMissingReport(job string, oemAggressive bool) string {
	base := fmt.Sprintf("no completion report found for job %q: the run either never started "+
		"(cron misconfigured / device asleep) or was terminated before finishing.", job)
	if oemAggressive {
		return base + " Aggressive OEM power management detected on this device — most likely cause: " +
			"the system killed Termux despite termux-wake-lock. Apply the checklist from 'hlops doctor' " +
			"(https://dontkillmyapp.com/)."
	}
	return base + " Check crond is running ('sv-status crond') and the logs under " +
		"$HOME/.local/state/hlops/logs."
}
