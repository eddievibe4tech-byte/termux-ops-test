package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/eddievibe4tech-byte/termux-ops/internal/bridge"
	"github.com/eddievibe4tech-byte/termux-ops/internal/sched"
)

// newRunCmd implements `hlops run backup|cleanup` — the scheduled entrypoint.
// It behaves like the standalone commands but is what cron jobs (and the
// Companion APK via the Termux RUN_COMMAND intent, issue #38) should call:
// it always takes the lock + wake-lock guards, and with --json it prints the
// normalized bridge status document instead of human output so callers can
// parse the result without reading report files themselves.
func newRunCmd() *cobra.Command {
	var (
		dryRun bool
		asJSON bool
		task   string
	)
	run := &cobra.Command{
		Use:   "run <backup|cleanup>",
		Short: "Run a job as intended for schedulers and the Companion APK (lock + wake-lock guarded)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "backup":
				if err := runBackup(cmd, dryRun, task, asJSON); err != nil {
					return err
				}
			case "cleanup":
				if err := runCleanup(cmd, dryRun, false, asJSON); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unknown job %q (want backup or cleanup)", args[0])
			}
			if asJSON {
				return emitBridgeStatus(cmd, args[0])
			}
			return nil
		},
	}
	f := run.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "do not write archives or delete anything")
	f.BoolVar(&asJSON, "json", false, "after the run, print the normalized bridge status JSON (schema v1)")
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
