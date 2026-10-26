package cli

import (
"fmt"

"github.com/spf13/cobra"

"github.com/eddievibe4tech-byte/termux-ops/internal/sched"
)

// newRunCmd implements `hlops run backup|cleanup` — the scheduled entrypoint.
// It behaves like the standalone commands but is what cron jobs should call,
// and it always takes the lock + wake-lock guards.
func newRunCmd() *cobra.Command {
run := &cobra.Command{
Use:   "run <backup|cleanup>",
Short: "Run a job as intended for schedulers (lock + wake-lock guarded)",
Args:  cobra.ExactArgs(1),
RunE: func(cmd *cobra.Command, args []string) error {
switch args[0] {
case "backup":
return runBackup(cmd, false, "", false)
case "cleanup":
return runCleanup(cmd, false, false, false)
default:
return fmt.Errorf("unknown job %q (want backup or cleanup)", args[0])
}
},
}
return run
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
