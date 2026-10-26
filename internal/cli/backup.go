package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/eddievibe4tech-byte/termux-ops/internal/backup"
	"github.com/eddievibe4tech-byte/termux-ops/internal/cleanup"
	"github.com/eddievibe4tech-byte/termux-ops/internal/lock"
	"github.com/eddievibe4tech-byte/termux-ops/internal/wakelock"
)

// newBackupCmd implements the standalone `hlops backup` command.
func newBackupCmd() *cobra.Command {
	var (
		dryRun bool
		task   string
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Run backup tasks once (single task with --task)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackup(cmd, dryRun, task, asJSON)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "do not write archives or delete anything")
	cmd.Flags().StringVar(&task, "task", "", "run only the named task")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the report as JSON")
	return cmd
}

// runBackup is shared by `hlops backup` and `hlops run backup`.
func runBackup(cmd *cobra.Command, dryRun bool, task string, asJSON bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if task != "" {
		if _, ok := cfg.TaskByName(task); !ok {
			return &exitErr{code: ExitError, msg: fmt.Sprintf("unknown task %q", task)}
		}
	}

	lk, err := lock.Acquire("backup")
	if err != nil {
		if errors.Is(err, lock.ErrLocked) {
			return &exitErr{code: ExitLocked, msg: err.Error()}
		}
		return err
	}
	defer lk.Release()

	wl := wakelock.Acquire()
	defer wl.Release()

	runner := &backup.Runner{Cfg: cfg, DryRun: dryRun, Only: task}
	rep := runner.RunAll()

	file, werr := backup.WriteReport(rep)
	if werr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not write report: %v\n", werr)
	}

	if asJSON {
		if err := encodeJSON(cmd.OutOrStdout(), rep); err != nil {
			return err
		}
	} else {
		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "backup: success=%d failed=%d skipped=%d dry_run=%t\n",
			rep.Success, rep.Failed, rep.Skipped, rep.DryRun)
		for _, r := range rep.Results {
			extra := r.Archive
			if r.Manifest != "" {
				extra += " +manifest"
			}
			fmt.Fprintf(w, "  %-7s %-12s %s %s\n", r.Status, r.Task, extra, r.Reason+r.Error)
		}
		if file != "" {
			fmt.Fprintf(w, "report: %s\n", file)
		}
	}

	if rep.Failed > 0 {
		return &exitErr{code: ExitError, msg: fmt.Sprintf("%d task(s) failed", rep.Failed)}
	}
	return nil
}

// newCleanupCmd implements the standalone `hlops cleanup` command.
func newCleanupCmd() *cobra.Command {
	var (
		dryRun         bool
		allowDangerous bool
		asJSON         bool
	)
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Run cleanup actions (dangerous actions are skipped by default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCleanup(cmd, dryRun, allowDangerous, asJSON)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list what would be removed without removing it")
	cmd.Flags().BoolVar(&allowDangerous, "allow-dangerous", false, "enable dangerous cleanup actions")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the report as JSON")
	return cmd
}

// runCleanup is shared by `hlops cleanup` and `hlops run cleanup`.
func runCleanup(cmd *cobra.Command, dryRun, allowDangerous, asJSON bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	lk, err := lock.Acquire("cleanup")
	if err != nil {
		if errors.Is(err, lock.ErrLocked) {
			return &exitErr{code: ExitLocked, msg: err.Error()}
		}
		return err
	}
	defer lk.Release()

	wl := wakelock.Acquire()
	defer wl.Release()

	planner := &cleanup.Planner{Cfg: cfg, DryRun: dryRun, AllowDangerous: allowDangerous}
	rep := planner.RunAll()

	if asJSON {
		return encodeJSON(cmd.OutOrStdout(), rep)
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "cleanup: dry_run=%t allow_dangerous=%t\n", rep.DryRun, allowDangerous)
	for _, r := range rep.Results {
		fmt.Fprintf(w, "  [%-26s] %-20s freed=%s %s%s\n",
			r.Status, r.ID, human(r.FreedBytes), r.CommandShown, r.Error)
	}
	return nil
}
