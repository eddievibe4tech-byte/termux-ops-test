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

// Interrupted reports whether the process received a termination signal.
// Set by WatchSignals; read by diagnostics in cli.Execute.
var Interrupted = false

// LastSignal holds the name of the termination signal received, if any.
var LastSignal string

// WatchSignals installs SIGINT/SIGTERM handlers. On receipt it logs a
// structured warning naming the signal and hinting at OEM task killers, then
// exits the process with 128+signum after best-effort cleanup via onClose.
// Call once at process start (main). onClose may be nil.
func WatchSignals(onClose func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-ch
		Interrupted = true
		sig, ok := s.(syscall.Signal)
		if !ok {
			sig = syscall.SIGTERM
		}
		LastSignal = s.String()
		slog.Warn("hlops received termination signal",
			"signal", s.String(),
			"hint", "if this was not you, an aggressive OEM battery manager "+
				"(ColorOS/MIUI/OriginOS) likely killed Termux despite the wake lock; "+
				"run 'hlops doctor' for the survival checklist")
		if onClose != nil {
			onClose()
		}
		os.Exit(ExitSignalBase + int(sig))
	}()
}

// ClassifyExit returns a one-line human diagnosis for a failed run, given the
// process exit code and whether a signal interrupted us. It never returns an
// empty string.
func ClassifyExit(code int, interrupted bool, oemAggressive bool) string {
	switch {
	case interrupted:
		msg := fmt.Sprintf("process was forcibly terminated (signal %s, exit %d)", LastSignal, code)
		if oemAggressive {
			return msg + ": known aggressive OEM power manager detected — see 'hlops doctor' checklist (dontkillmyapp.com)"
		}
		return msg + ": if unexpected, check OEM battery settings (see docs/termux-quickstart.md)"
	case code == ExitSignalBase+int(syscall.SIGKILL):
		// SIGKILL cannot be caught, so WatchSignals never fires; still worth
		// surfacing the interpretation when the *parent* reports it.
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
