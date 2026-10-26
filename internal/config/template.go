package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eddievibe4tech-byte/termux-ops/internal/paths"
)

// Template renders a starter config.toml using the real $HOME of the user.
func Template() string {
	home := paths.Home()
	dest := filepath.Join(home, "hlops-backups")
	if s := paths.SharedStorage(); s != "" {
		dest = filepath.Join(s, "hlops-backups")
	}
	var b strings.Builder
	b.WriteString("# hlops configuration\n")
	b.WriteString("# Docs: see README.md / docs/termux-quickstart.md\n\n")
	b.WriteString("[global]\n")
	b.WriteString(fmt.Sprintf("state_dir = %q\n\n", filepath.Join(home, ".local/state/hlops")))
	b.WriteString("[backup]\n")
	b.WriteString(fmt.Sprintf("destination = %q\n\n", dest))
	b.WriteString("[backup.retention]\n")
	b.WriteString("keep = 8            # keep at most 8 archives per task\n")
	b.WriteString("retention_days = 0 # 0 disables age-based deletion\n\n")
	b.WriteString("# Task 1: user scripts\n")
	b.WriteString("[[backup.tasks]]\n")
	b.WriteString("name = \"scripts\"\n")
	b.WriteString("type = \"files\"\n")
	b.WriteString(fmt.Sprintf("source = %q\n", filepath.Join(home, "scripts")))
	b.WriteString("exclude = [\"*.tmp\", \"node_modules\"]\n\n")
	b.WriteString("# Task 2: dotfiles (a single file works too)\n")
	b.WriteString("[[backup.tasks]]\n")
	b.WriteString("name = \"dotfiles\"\n")
	b.WriteString("type = \"files\"\n")
	b.WriteString(fmt.Sprintf("source = %q\n", filepath.Join(home, ".bashrc")))
	b.WriteString("exclude = [\"*.bak\"]\n\n")
	b.WriteString("# Task 3: Termux environment (packages, env vars, ~/.termux)\n")
	b.WriteString("[[backup.tasks]]\n")
	b.WriteString("name = \"termux-env\"\n")
	b.WriteString("type = \"termux-env\"\n")
	b.WriteString("# source is optional for termux-env; defaults to ~/.termux\n\n")
	b.WriteString("[analyze]\n")
	b.WriteString("top_n = 15\n")
	b.WriteString("timeout = 120\n\n")
	b.WriteString("[cleanup.native]\n")
	b.WriteString("tmp_enabled = true\n")
	b.WriteString("tmp_older_than = 7      # days\n")
	b.WriteString("old_backups_enabled = true\n\n")
	b.WriteString("[cleanup.providers]\n")
	b.WriteString("pkg = true   # Termux pkg cache (skipped gracefully when absent)\n")
	b.WriteString("apt = false  # Debian-style apt-get clean\n\n")
	b.WriteString("[logging]\n")
	b.WriteString("level = \"info\"\n")
	b.WriteString("# file = \"-\"  # uncomment to disable file logging\n")
	return b.String()
}

// InitConfig writes the template to target (default: ~/.config/hlops/config.toml).
// It refuses to overwrite unless force=true. Returns the written path.
func InitConfig(target string, force bool) (string, error) {
	if target == "" {
		target = paths.ConfigFile()
	}
	if _, err := os.Stat(target); err == nil && !force {
		return target, fmt.Errorf("config already exists at %s (use --force to overwrite)", target)
	}
	if err := paths.EnsureDir(filepath.Dir(target)); err != nil {
		return target, err
	}
	if err := os.WriteFile(target, []byte(Template()), 0o600); err != nil {
		return target, err
	}
	return target, nil
}
