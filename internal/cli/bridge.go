package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/eddievibe4tech-byte/termux-ops/internal/bridge"
	"github.com/eddievibe4tech-byte/termux-ops/internal/paths"
)

// newBridgeCmd implements `hlops bridge ...` — the machine-facing surface for
// the future Companion APK (issue #38). Everything here is stable JSON with a
// schema version so the App can evolve independently of human output.
func newBridgeCmd() *cobra.Command {
	bridgeCmd := &cobra.Command{
		Use:   "bridge",
		Short: "Machine-readable interface for the Companion APK (issue #37/#38)",
		Long: `Commands consumed by the Android Companion App instead of parsing
human output: normalized run status from the newest JSON report, and the
latest log tail. Output is stable JSON guarded by a "schema" field.`,
	}
	bridgeCmd.AddCommand(newBridgeStatusCmd(), newBridgeLogsCmd())
	return bridgeCmd
}

func newBridgeStatusCmd() *cobra.Command {
	var job string
	cmd := &cobra.Command{
		Use:   "status",
		Short: fmt.Sprintf("Print the latest job status as bridge JSON (schema v%d)", bridge.SchemaVersion),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if job != "" && job != bridge.JobBackup && job != bridge.JobCleanup {
				return fmt.Errorf("unknown job %q (want backup or cleanup)", job)
			}
			info, err := bridge.LatestReport(job)
			if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
				// Fresh install / never ran: still valid JSON, HTTP-200 semantics.
				return encodeJSON(cmd.OutOrStdout(), bridge.NoReport(Version))
			}
			if err != nil {
				return err
			}
			bs, err := bridge.LoadStatus(info, Version)
			if err != nil {
				return err
			}
			return encodeJSON(cmd.OutOrStdout(), bs)
		},
	}
	cmd.Flags().StringVar(&job, "job", "", "filter by job kind: backup|cleanup (default: newest of any)")
	return cmd
}

func newBridgeLogsCmd() *cobra.Command {
	var lines int
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Tail the newest hlops log file as JSON lines for the APK viewer",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := paths.LogDir()
			out := struct {
				File      string   `json:"file"`
				Lines     []string `json:"lines"`
				Truncated bool     `json:"truncated"`
			}{Lines: []string{}}
			f, err := newestFile(dir, "hlops", ".log")
			if err != nil {
				return encodeJSON(cmd.OutOrStdout(), out) // empty but valid
			}
			out.File = f
			data, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			all := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
			if len(all) == 1 && all[0] == "" {
				// Empty file: keep the "array, never null" JSON invariant.
				all = []string{}
			}
			if len(all) > lines {
				out.Truncated = true
				all = all[len(all)-lines:]
			}
			out.Lines = all
			return encodeJSON(cmd.OutOrStdout(), out)
		},
	}
	cmd.Flags().IntVar(&lines, "lines", 200, "maximum number of trailing lines to return")
	return cmd
}

// newestFile returns the most recently modified regular file in dir whose
// name starts with prefix and ends with suffix.
func newestFile(dir, prefix, suffix string) (string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var best string
	var bestMod time.Time
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasPrefix(n, prefix) || !strings.HasSuffix(n, suffix) {
			continue
		}
		fi, ierr := e.Info()
		if ierr != nil {
			continue
		}
		if fi.ModTime().After(bestMod) {
			bestMod = fi.ModTime()
			best = filepath.Join(dir, n)
		}
	}
	if best == "" {
		return "", fs.ErrNotExist
	}
	return best, nil
}
