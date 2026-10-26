package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/eddievibe4tech-byte/termux-ops/internal/config"
	"github.com/eddievibe4tech-byte/termux-ops/internal/paths"
	"github.com/eddievibe4tech-byte/termux-ops/internal/termux"
)

// encodeJSON pretty-prints v as JSON to w.
func encodeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// newInitConfigCmd implements `hlops init-config`.
func newInitConfigCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init-config",
		Short: "Create a starter config with scripts/dotfiles/termux-env tasks",
		Long: `Writes a documented example config to $HOME/.config/hlops/config.toml
(or --config path). Existing files are never overwritten unless --force.
Example tasks use the real current $HOME paths.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ro.configFile
			if target == "" {
				target = paths.ConfigFile()
			}
			written, err := config.InitConfig(target, force)
			if err != nil {
				return &exitErr{code: ExitError, msg: err.Error()}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", written)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing config file")
	return cmd
}

// newDoctorCmd implements `hlops doctor`.
func newDoctorCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Show environment, paths and config health",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			info := termux.Detect()
			cfgPath := ro.configFile
			if cfgPath == "" {
				if env := os.Getenv("HLOPS_CONFIG"); env != "" {
					cfgPath = env
				} else {
					cfgPath = paths.ConfigFile()
				}
			}
			_, statErr := os.Stat(cfgPath)
			exists := statErr == nil

			cfg, err := loadConfig()
			configOK := err == nil
			var configErr string
			if err != nil {
				configErr = err.Error()
			}

			checks := []map[string]any{
				{"name": "config_file_exists", "ok": exists, "detail": cfgPath},
				{"name": "config_parses", "ok": configOK, "detail": configErr},
				{"name": "config_dir_writable", "ok": paths.Writable(paths.ConfigDir()), "detail": paths.ConfigDir()},
				{"name": "state_dir_writable", "ok": paths.Writable(paths.StateDir()), "detail": paths.StateDir()},
			}
			allOK := true
			for _, c := range checks {
				if ok, _ := c["ok"].(bool); !ok {
					allOK = false
				}
			}

			report := map[string]any{
				"version":        Version,
				"is_termux":      info.IsTermux,
				"termux_version": info.Version,
				"prefix":         info.Prefix,
				"home":           paths.Home(),
				"shared_storage": paths.SharedStorage(),
				"config_path":    cfgPath,
				"log_dir":        paths.LogDir(),
				"lock_dir":       paths.LockDir(),
				"checks":         checks,
				"healthy":        allOK,
			}
			if configOK && cfg != nil {
				tasks := make([]string, 0, len(cfg.Backup.Tasks))
				for _, t := range cfg.Backup.Tasks {
					st := "enabled"
					if !t.IsEnabled() {
						st = "disabled"
					}
					tasks = append(tasks, fmt.Sprintf("%s (%s, type=%s)", t.Name, st, t.Type))
				}
				report["backup_tasks"] = tasks
			}

			if asJSON {
				return encodeJSON(cmd.OutOrStdout(), report)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "hlops doctor\n")
			fmt.Fprintf(w, "  version:        %s\n", Version)
			fmt.Fprintf(w, "  termux:         %t (%s)\n", info.IsTermux, orDash(info.Version))
			fmt.Fprintf(w, "  home:           %s\n", paths.Home())
			fmt.Fprintf(w, "  prefix:         %s\n", orDash(info.Prefix))
			fmt.Fprintf(w, "  shared storage: %s\n", orDash(paths.SharedStorage()))
			fmt.Fprintf(w, "  config path:    %s\n", cfgPath)
			fmt.Fprintf(w, "  log dir:        %s\n", paths.LogDir())
			fmt.Fprintf(w, "  lock dir:       %s\n", paths.LockDir())
			for _, c := range checks {
				status := "FAIL"
				if ok, _ := c["ok"].(bool); ok {
					status = "ok"
				}
				fmt.Fprintf(w, "  [%-4s] %-22s %v\n", status, c["name"], c["detail"])
			}
			if tasks, ok := report["backup_tasks"].([]string); ok {
				fmt.Fprintf(w, "  backup tasks:\n")
				for _, t := range tasks {
					fmt.Fprintf(w, "    - %s\n", t)
				}
			}
			fmt.Fprintf(w, "  healthy: %t\n", allOK)
			if !allOK {
				return &exitErr{code: ExitError, msg: "doctor found problems"}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output in JSON format")
	return cmd
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
