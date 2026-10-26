package cli

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/eddievibe4tech-byte/termux-ops/internal/bridge"
	"github.com/eddievibe4tech-byte/termux-ops/internal/sched"
	"github.com/eddievibe4tech-byte/termux-ops/internal/termination"
	"github.com/eddievibe4tech-byte/termux-ops/internal/termux"
)

// newRunCmd implements `hlops run backup|cleanup` — the scheduled entrypoint.
// It behaves like the standalone commands but is what cron jobs (and the
// Companion APK via the Termux RUN_COMMAND intent, issue #38) should call:
// it always takes the lock + wake-lock guards, and with --json --bridge it
// prints exactly one normalized BridgeStatus document for every outcome —
// including hard failures that never wrote a report — so the App can parse
// stdout without reading report files or special-casing empty output.
func newRunCmd() *cobra.Command {
	var (
		dryRun     bool
		asJSON     bool
		bridgeJSON bool
		task       string
	)
	run := &cobra.Command{
		Use:   "run <backup|cleanup>",
		Short: "Run a job as intended for schedulers and the Companion APK (lock + wake-lock guarded)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			job := args[0]
			switch job {
			case "backup":
				if err := runBackup(cmd, dryRun, task, asJSON); err != nil {
					return failBridge(cmd, job, asJSON && bridgeJSON, err)
				}
			case "cleanup":
				if err := runCleanup(cmd, dryRun, false, asJSON); err != nil {
					return failBridge(cmd, job, asJSON && bridgeJSON, err)
				}
			default:
				return fmt.Errorf("unknown job %q (want backup or cleanup)", job)
			}
			if asJSON && bridgeJSON {
				if err := emitBridgeStatus(cmd, job); err != nil {
					return failBridge(cmd, job, true, err)
				}
			} else if asJSON {
				// Non-bridge --json consumers still benefit from the issue #43
				// fault-tolerance hint: when the report is missing (killed mid
				// run / never started), log why instead of staying silent.
				if _, err := bridge.LatestReport(job); err != nil {
					oemAgg := termux.Detect().IsTermux && termux.DetectOEM().Aggressive
					slog.Warn("missing completion report", "job", job,
						"diagnosis", termination.DiagnoseMissingReport(job, oemAgg))
				}
			}
			return nil
		},
	}
	f := run.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "do not write archives or delete anything")
	f.BoolVar(&asJSON, "json", false, "after the run, print the job result as JSON")
	f.BoolVar(&bridgeJSON, "bridge", false, "with --json: emit the normalized bridge status (schema v1) instead of the raw report")
	f.StringVar(&task, "task", "", "backup only: run only the named task")
	return run
}

// emitBridgeStatus re-reads the just-written report through the bridge layer
// so `hlops run backup --json` output matches `hlops bridge status --json`.
func emitBridgeStatus(cmd *cobra.Command, job string) error {
	info, err := bridge.LatestReport(job)
	if err != nil {
		return err
	}
	bs, err := bridge.LoadStatus(info, Version)
	if err != nil {
		return err
	}
	return encodeJSON(cmd.OutOrStdout(), bs)
}

// failBridge keeps the machine contract whole: when a --json run fails so
// hard that no report was written (lock contention, invalid config, ...),
// or when the just-written report cannot be re-read, the APK still receives
// one parseable BridgeStatus document with status="failed" and an error
// message on stdout — plus the original non-zero exit code. Without this,
// callers would have to special-case "exit != 0 && empty stdout" and risk
// falling back to a stale report instead.
func failBridge(cmd *cobra.Command, job string, asJSON bool, runErr error) error {
	if asJSON {
		_ = encodeJSON(cmd.OutOrStdout(), bridge.NewFailedStatus(job, Version, runErr.Error()))
	}
	return runErr
}

// newScheduleCmd implements `hlops schedule generate cron [--job backup|cleanup]`.
func newScheduleCmd() *cobra.Command {
	var (
		timeExpr string
		binary   string
		noWake   bool
		job      string
	)

	genCron := &cobra.Command{
		Use:   "cron",
		Short: "Generate a crontab line for 'hlops run <job>'",
		Long: `Generates a valid cron entry that wraps hlops run with
termux-wake-lock / termux-wake-unlock on Termux, plus installation
instructions for crond (cronie) and battery-optimisation notes.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := sched.GenerateCron(sched.CronOptions{
				Job:      job,
				Time:     timeExpr,
				Binary:   binary,
				WakeLock: !noWake,
			})
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
	genCron.Flags().StringVar(&timeExpr, "time", "0 3 * * *", "5-field cron expression, e.g. \"0 2 * * *\"")
	genCron.Flags().StringVar(&binary, "binary", "", "path to the hlops binary (default: auto-detect)")
	genCron.Flags().BoolVar(&noWake, "no-wakelock", false, "do not wrap with termux-wake-lock")
	genCron.Flags().StringVar(&job, "job", "backup", "job name: backup|cleanup")

	generate := &cobra.Command{
		Use:   "generate",
		Short: "Generate scheduler entries",
	}
	generate.AddCommand(genCron)

	schedule := &cobra.Command{
		Use:   "schedule",
		Short: "Manage scheduled runs (generator only; no auto-install in v1)",
	}
	schedule.AddCommand(generate)
	return schedule
}
