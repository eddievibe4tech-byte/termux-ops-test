// Package paths resolves cross-platform (Termux / Linux) directories for hlops.
package paths

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/eddievibe4tech-byte/termux-ops/internal/termux"
)

// AppName is the application identifier used in XDG-style directories.
const AppName = "hlops"

// Home returns the effective home directory. On Termux it prefers $HOME
// (which already points to /data/data/com.termux/files/home); on plain
// Linux it falls back to os.UserHomeDir. Nothing is hard-coded.
func Home() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// ConfigDir returns ~/.config/hlops (created lazily by callers when needed).
func ConfigDir() string { return filepath.Join(Home(), ".config", AppName) }

// ConfigFile returns the default config file path inside ConfigDir.
func ConfigFile() string { return filepath.Join(ConfigDir(), "config.toml") }

// StateDir returns $HOME/.local/state/hlops.
func StateDir() string { return filepath.Join(Home(), ".local", "state", AppName) }

// LogDir returns $HOME/.local/state/hlops/logs.
func LogDir() string { return filepath.Join(StateDir(), "logs") }

// LockDir returns $HOME/.local/state/hlops/locks.
func LockDir() string { return filepath.Join(StateDir(), "locks") }

// ReportDir returns $HOME/.local/state/hlops/reports.
func ReportDir() string { return filepath.Join(StateDir(), "reports") }

// SharedStorage returns ~/storage/shared if it exists (Termux after
// termux-setup-storage), otherwise empty string.
func SharedStorage() string { return termux.Detect().SharedStorage }

// Resolve expands a leading "~" or "$HOME" to the real home directory and
// cleans the result. It understands "~/storage/shared" style paths on both
// Termux and Linux without hard-coding absolute prefixes.
func Resolve(p string) string {
	if p == "" {
		return ""
	}
	home := Home()
	switch {
	case p == "~":
		p = home
	case strings.HasPrefix(p, "~/"):
		p = filepath.Join(home, p[2:])
	case strings.HasPrefix(p, "$HOME/"):
		p = filepath.Join(home, p[len("$HOME/"):])
	case p == "$HOME":
		p = home
	}
	if !filepath.IsAbs(p) {
		wd, err := os.Getwd()
		if err == nil {
			p = filepath.Join(wd, p)
		}
	}
	return filepath.Clean(p)
}

// EnsureDir creates dir (and parents) with 0700 if missing.
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0o700)
}

// Writable reports whether dir can be written (creates a temp probe file).
func Writable(dir string) bool {
	if err := EnsureDir(dir); err != nil {
		return false
	}
	f := filepath.Join(dir, ".hlops-write-probe")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		return false
	}
	_ = os.Remove(f)
	return true
}
