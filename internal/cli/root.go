// Package cli wires the cobra command tree for hlops. Every command
// supports --help; global flags configure logging and config location.
package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/eddievibe4tech-byte/termux-ops/internal/config"
	"github.com/eddievibe4tech-byte/termux-ops/internal/logging"
	"github.com/eddievibe4tech-byte/termux-ops/internal/paths"
	"github.com/eddievibe4tech-byte/termux-ops/internal/termination"
	"github.com/eddievibe4tech-byte/termux-ops/internal/termux"
)

// Version is stamped via -ldflags at build time.
var Version = "dev"

// Exit codes used across commands.
const (
	ExitOK     = 0
	ExitError  = 1
	ExitSyntax = config.ExitCodeSyntax // 2 – bad config file
	ExitLocked = 6                     // 5/6 – another instance holds the lock
)

type rootOpts struct {
	configFile string
	logFile    string
	logLevel   string
	quiet      bool
	noColor    bool
}

var ro rootOpts

// NewRootCmd builds the full command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "hlops",
		Short:         "Termux-friendly operations tool: analyze, backup, cleanup, run, schedule",
		Long:          "hlops is a single-binary operations helper optimised for Termux on Android,\nalso usable on regular Linux.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cleanup, err := logging.Setup(logging.Options{
				Level:   ro.logLevel,
				File:    ro.logFile,
				Quiet:   ro.quiet,
				NoColor: ro.noColor || !logging.IsTerminal(os.Stderr),
				Prefix:  cmd.Name(),
			})
			if err != nil {
				return err
			}
			closers = append(closers, cleanup)
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) { closeAll() },
	}

	pf := root.PersistentFlags()
	pf.StringVar(&ro.configFile, "config", "", "path to config file (default $HOME/.config/hlops/config.toml, env HLOPS_CONFIG)")
	pf.StringVar(&ro.logFile, "log-file", "", "log file path (\"-\" disables file logging)")
	pf.StringVar(&ro.logLevel, "log-level", "info", "log level: debug|info|warn|error")
	pf.BoolVar(&ro.quiet, "quiet", false, "suppress stderr log output")
	pf.BoolVar(&ro.noColor, "no-color", false, "disable coloured log output")

	root.AddCommand(
		newVersionCmd(),
		newInitConfigCmd(),
		newDoctorCmd(),
		newAnalyzeCmd(),
		newBackupCmd(),
		newCleanupCmd(),
		newRunCmd(),
		newScheduleCmd(),
		newBridgeCmd(),
	)
	return root
}

var closers []func()

func closeAll() {
	for _, f := range closers {
		f()
	}
	closers = nil
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	defer closeAll()
	// Issue #43: on aggressive OEM ROMs the system task killer sends
	// SIGTERM/SIGKILL to Termux despite wake locks. WatchSignals turns a
	// catchable termination into a logged, diagnosable exit (128+signum)
	// instead of a silent death that looks like "the backup crashed". The
	// diagnosis is printed inside the signal goroutine before os.Exit —
	// nothing after this function would ever run on that path.
	oemAggressive := func() bool {
		return termux.Detect().IsTermux && termux.DetectOEM().Aggressive
	}
	termination.WatchSignals(closeAll, oemAggressive)
	cmd := NewRootCmd()
	if err := cmd.Execute(); err != nil {
		var ee *exitErr
		code := ExitError
		if errors.As(err, &ee) {
			code = ee.code
		} else if errors.Is(err, config.ErrConfigSyntax) {
			code = ExitSyntax
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		// One-line diagnosis so users (and log scrapers) can tell a real
		// crash apart from a forced system termination or a benign skip.
		// interrupted=false here: signal deaths exit inside WatchSignals
		// and never reach this branch.
		slog.Warn("run terminated", "exit_code", code,
			"diagnosis", termination.ClassifyExit(code, false, oemAggressive()))
		return code
	}
	return ExitOK
}

// exitErr carries an explicit process exit code.
type exitErr struct {
	code int
	msg  string
}

func (e *exitErr) Error() string { return e.msg }

func loadConfig() (*config.Config, error) {
	return config.Load(ro.configFile)
}

// newVersionCmd implements `hlops version`.
func newVersionCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version, OS, arch and Termux detection",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			info := termux.Detect()
			if asJSON {
				out := map[string]any{
					"version":   Version,
					"goos":      runtime.GOOS,
					"goarch":    runtime.GOARCH,
					"is_termux": info.IsTermux,
					"prefix":    info.Prefix,
					"home":      paths.Home(),
				}
				return encodeJSON(cmd.OutOrStdout(), out)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "hlops %s\n", Version)
			fmt.Fprintf(w, "goos: %s\n", runtime.GOOS)
			fmt.Fprintf(w, "goarch: %s\n", runtime.GOARCH)
			fmt.Fprintf(w, "termux: %t\n", info.IsTermux)
			if info.IsTermux {
				fmt.Fprintf(w, "TERMUX_VERSION: %s\n", info.Version)
				fmt.Fprintf(w, "PREFIX: %s\n", info.Prefix)
			}
			fmt.Fprintf(w, "home: %s\n", paths.Home())
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output in JSON format")
	return cmd
}
