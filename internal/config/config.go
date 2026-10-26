// Package config loads hlops TOML configuration with multi-task backup
// support, defaults, and validation. Syntax errors are reported via
// ErrConfigSyntax so the CLI can exit with code 2. HLOPS_CONFIG overrides
// the default config path.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/eddievibe4tech-byte/termux-ops/internal/paths"
)

// ExitCodeSyntax is the process exit code for configuration syntax errors.
const ExitCodeSyntax = 2

// ErrConfigSyntax marks a TOML parse failure.
var ErrConfigSyntax = errors.New("config syntax error")

// Task types.
const (
	TypeFiles     = "files"      // generic tar.gz of a directory/file
	TypeTermuxEnv = "termux-env" // package list + env + ~/.termux
)

// Retention is a per-task or global retention policy.
type Retention struct {
	Keep           int `toml:"keep"`            // keep at most N archives per task (0 = unlimited)
	RetentionDays  int `toml:"retention_days"`  // delete archives older than N days (0 = off)
}

// Task describes one backup job.
type Task struct {
	Name        string    `toml:"name"`
	Type        string    `toml:"type"` // files | termux-env
	Source      string    `toml:"source"`
	Destination string    `toml:"destination"`
	Exclude     []string  `toml:"exclude"`
	Enabled     *bool     `toml:"enabled"` // nil => true
	Retention   *Retention `toml:"retention"`
}

// IsEnabled reports whether the task participates in runs.
func (t Task) IsEnabled() bool { return t.Enabled == nil || *t.Enabled }

// Backup is the [backup] table.
type Backup struct {
	Destination string    `toml:"destination"`
	Tasks       []Task    `toml:"tasks"`
	Retention   Retention `toml:"retention"`
}

// Analyze is the [analyze] table.
type Analyze struct {
	Targets []string `toml:"targets"` // empty => home (+ shared storage on Termux)
	TopN    int      `toml:"top_n"`   // default 15
	Timeout int      `toml:"timeout"` // seconds, default 120
}

// CleanupNative configures native cleanup actions.
type CleanupNative struct {
	TmpEnabled   *bool `toml:"tmp_enabled"`   // nil => true
	TmpOlderThan int   `toml:"tmp_older_than"` // days, default 7
	OldBackupsEnabled *bool `toml:"old_backups_enabled"` // nil => true
}

// CleanupProviders lists enabled provider cleanups.
type CleanupProviders struct {
	Pkg bool `toml:"pkg"`
	Apt bool `toml:"apt"`
}

// Cleanup is the [cleanup] table.
type Cleanup struct {
	Native    CleanupNative    `toml:"native"`
	Providers CleanupProviders `toml:"providers"`
}

// Logging is the [logging] table.
type Logging struct {
	Level string `toml:"level"` // debug|info|warn|error
	File  string `toml:"file"`  // "" => default, "-" => disable
}

// Config is the whole document.
type Config struct {
	Global  GlobalConfig     `toml:"global"`
	Backup  Backup           `toml:"backup"`
	Analyze Analyze          `toml:"analyze"`
	Cleanup Cleanup          `toml:"cleanup"`
	Logging Logging          `toml:"logging"`

	// Path records where the config was loaded from ("" if defaults).
	Path string `toml:"-"`
}

// GlobalConfig holds misc settings.
type GlobalConfig struct {
	StateDir string `toml:"state_dir"` // override $HOME/.local/state/hlops
}

// Defaults returns a Config with sane built-in defaults applied.
func Defaults() *Config {
	home := paths.Home()
	dest := filepath.Join(home, "hlops-backups")
	if s := paths.SharedStorage(); s != "" {
		dest = filepath.Join(s, "hlops-backups")
	}
	c := &Config{}
	c.Backup.Destination = dest
	c.Backup.Retention = Retention{Keep: 8, RetentionDays: 0}
	c.Analyze.TopN = 15
	c.Analyze.Timeout = 120
	c.Cleanup.Native.TmpOlderThan = 7
	yes := true
	c.Cleanup.Native.TmpEnabled = &yes
	c.Cleanup.Native.OldBackupsEnabled = &yes
	c.Logging.Level = "info"
	return c
}

// Load reads the config file. Resolution order:
// explicit --config flag > $HLOPS_CONFIG > ~/.config/hlops/config.toml.
// A missing file yields Defaults() with no error; a syntax error yields
// an error wrapping ErrConfigSyntax.
func Load(explicit string) (*Config, error) {
	file := explicit
	if file == "" {
		file = os.Getenv("HLOPS_CONFIG")
	}
	if file == "" {
		file = paths.ConfigFile()
	}
	cfg := Defaults()
	data, err := os.ReadFile(file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil // no config file => defaults
		}
		return cfg, fmt.Errorf("read config %s: %w", file, err)
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		var derr *toml.DecodeError
		if errors.As(err, &derr) {
			row, col := derr.Position()
			return cfg, fmt.Errorf("%w: %s:%d:%d: %v", ErrConfigSyntax, file, row, col, derr)
		}
		return cfg, fmt.Errorf("%w: %s: %v", ErrConfigSyntax, file, err)
	}
	cfg.Path = file
	normalize(cfg)
	return cfg, nil
}

func normalize(c *Config) {
	def := Defaults()
	if c.Backup.Destination == "" {
		c.Backup.Destination = def.Backup.Destination
	}
	if c.Backup.Retention.Keep == 0 && c.Backup.Retention.RetentionDays == 0 {
		c.Backup.Retention = def.Backup.Retention
	}
	if c.Analyze.TopN <= 0 {
		c.Analyze.TopN = def.Analyze.TopN
	}
	if c.Analyze.Timeout <= 0 {
		c.Analyze.Timeout = def.Analyze.Timeout
	}
	if c.Cleanup.Native.TmpOlderThan <= 0 {
		c.Cleanup.Native.TmpOlderThan = def.Cleanup.Native.TmpOlderThan
	}
	if len(c.Backup.Tasks) == 0 {
		c.Backup.Tasks = DefaultTasks()
	}
	for i := range c.Backup.Tasks {
		t := &c.Backup.Tasks[i]
		if t.Type == "" {
			t.Type = TypeFiles
		}
		if t.Destination == "" {
			t.Destination = c.Backup.Destination
		}
		if t.Name == "" {
			t.Name = fmt.Sprintf("task-%d", i+1)
		}
	}
}

// DefaultTasks returns the built-in example tasks used when no config exists
// and by init-config. Paths use the real $HOME of the current user.
func DefaultTasks() []Task {
	home := paths.Home()
	yes := true
	no := false
	tasks := []Task{
		{Name: "scripts", Type: TypeFiles, Source: filepath.Join(home, "scripts"), Enabled: &yes},
		{Name: "dotfiles", Type: TypeFiles, Source: filepath.Join(home, ".bashrc"), Enabled: &yes,
			Exclude: []string{"*.bak"}},
		{Name: "termux-env", Type: TypeTermuxEnv, Enabled: &yes},
	}
	// On non-Termux systems the termux-env task stays but is disabled by
	// default so it reports skipped instead of failing.
	if !isTermuxHome(home) {
		tasks[2].Enabled = &no
	}
	return tasks
}

func isTermuxHome(home string) bool {
	return len(home) > 0 && (filepath.Dir(home) == "/data/data/com.termux/files")
}

// TaskByName finds a task by name; ok=false when not found.
func (c *Config) TaskByName(name string) (Task, bool) {
	for _, t := range c.Backup.Tasks {
		if t.Name == name {
			return t, true
		}
	}
	return Task{}, false
}

// EffectiveRetention merges task-level retention over the global default.
func (c *Config) EffectiveRetention(t Task) Retention {
	if t.Retention != nil {
		r := *t.Retention
		if r.Keep == 0 && r.RetentionDays == 0 {
			return c.Backup.Retention
		}
		return r
	}
	return c.Backup.Retention
}
