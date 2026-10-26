package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/eddievibe4tech-byte/termux-ops/internal/analyze"
	"github.com/eddievibe4tech-byte/termux-ops/internal/paths"
)

// newAnalyzeCmd implements `hlops analyze`.
func newAnalyzeCmd() *cobra.Command {
	var (
		asJSON  bool
		timeout time.Duration
		topN    int
		shared  bool
	)
	cmd := &cobra.Command{
		Use:   "analyze [path...]",
		Short: "Scan storage usage of $HOME and shared storage",
		Long: `Analyze scans the given paths (default: $HOME). It never follows
symlinks, skips /proc, /sys and $PREFIX system dirs, tolerates permission
errors and honours --timeout. Use --shared to also scan ~/storage/shared.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			roots := args
			if len(roots) == 0 {
				roots = []string{paths.Home()}
				if shared {
					if s := paths.SharedStorage(); s != "" {
						if _, statErr := os.Stat(s); statErr == nil {
							roots = append(roots, s)
						} else if asJSON {
							// reported via skipped roots below
						}
					}
				}
			}
			if topN <= 0 {
				topN = cfg.Analyze.TopN
			}
			if timeout == 0 {
				timeout = time.Duration(cfg.Analyze.Timeout) * time.Second
			}

			ctx := context.Background()
			results := make([]*analyze.Result, 0, len(roots))
			for _, r := range roots {
				res, err := analyze.Scan(ctx, filepath.Clean(r), analyze.Options{TopN: topN, Timeout: timeout})
				if err != nil {
					return &exitErr{code: ExitError, msg: fmt.Sprintf("scan %s: %v", r, err)}
				}
				results = append(results, res)
			}

			if asJSON {
				return encodeJSON(cmd.OutOrStdout(), map[string]any{"results": results})
			}
			w := cmd.OutOrStdout()
			for _, res := range results {
				fmt.Fprintf(w, "== %s ==\n", res.Root)
				fmt.Fprintf(w, "total: %s  files: %d  dirs: %d  (%.1fs%s)\n",
					human(res.TotalSize), res.FileCount, res.DirCount, res.ScanSeconds,
					map[bool]string{true: ", TIMED OUT"}[res.TimedOut])
				for _, v := range res.Volumes {
					if v.Error != "" {
						fmt.Fprintf(w, "volume %s: %s\n", v.Path, v.Error)
						continue
					}
					fmt.Fprintf(w, "volume %s: free %s / total %s (available %s)\n",
						v.Path, human(int64(v.Available)), human(int64(v.TotalBytes)), human(int64(v.Available)))
				}
				fmt.Fprintf(w, "top %d directories:\n", min(topN, len(res.Directories)))
				for _, e := range res.Directories {
					fmt.Fprintf(w, "  %12s  %s\n", human(e.Size), e.Path)
				}
				fmt.Fprintf(w, "top %d files:\n", min(topN, len(res.Files)))
				for _, e := range res.Files {
					fmt.Fprintf(w, "  %12s  %s\n", human(e.Size), e.Path)
				}
				if len(res.Skipped) > 0 {
					fmt.Fprintf(w, "skipped (%d unreadable entries):\n", len(res.Skipped))
					for i, s := range res.Skipped {
						if i >= 10 {
							fmt.Fprintf(w, "  ... %d more\n", len(res.Skipped)-10)
							break
						}
						fmt.Fprintf(w, "  %s (%s)\n", s.Path, s.Reason)
					}
				}
				fmt.Fprintln(w)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output in JSON format")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "scan timeout (default from config)")
	cmd.Flags().IntVar(&topN, "top", 0, "number of top entries to list (default from config)")
	cmd.Flags().BoolVar(&shared, "shared", false, "also scan shared storage (~/storage/shared)")
	return cmd
}

func human(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
