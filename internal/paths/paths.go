// Package paths resolves cross-platform (Termux / Linux) directories for hlops.
package paths

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"

	"github.com/eddievibe4tech-byte/termux-ops/internal/termux"
)

// AppName is the application identifier used in XDG-style directories.
const AppName = "hlops"

// Home returns the effective home directory. On Termux it prefers $HOME
// (which already points to /data/data/com.termux/files/home); on plain
// Linux it falls back to os.UserHomeDir. Nothing is hard-coded.
//
// The result is memoised on first call: hlops treats $HOME as fixed for the
// lifetime of a process (every state/config/log location derives from it), so
// a stable snapshot also keeps all path consumers consistent. Tests that must
// change HOME use SetHomeForTest.
var homeOnce struct {
	once sync.Once
	val  string
}

func Home() string {
	homeOnce.once.Do(func() {
		if h := os.Getenv("HOME"); h != "" {
			homeOnce.val = h
			return
		}
		h, err := os.UserHomeDir()
		if err != nil {
			return
		}
		homeOnce.val = h
	})
	return homeOnce.val
}

// SetHomeForTest overrides the memoised home value. It exists solely so unit
// tests can exercise home-dependent logic with a temporary directory; calling
// it from production code is a bug.
func SetHomeForTest(h string) {
	homeOnce.once = sync.Once{}
	homeOnce.once.Do(func() { homeOnce.val = h })
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

// HomeDisplay returns a display-friendly home prefix. The "~" form is only
// meaningful inside the shell that owns $HOME (Termux, desktop Linux), so it
// is used exactly there: when the process runs inside Termux ($PREFIX points
// under the Termux app dir) or when $HOME matches the OS account's home from
// the user database. Anywhere else — most importantly the Companion APK's own
// sandboxed process, whose HOME is /data/data/dev.hlops.companion while the
// hlops files live under Termux's home — the absolute home is returned so UI
// strings can never show a "~" that resolves to the wrong directory.
func HomeDisplay() string {
	h := Home()
	if h == "" {
		return "~"
	}
	if termux.Detect().IsTermux {
		return "~"
	}
	if uh := passwdHome(); uh != "" && filepath.Clean(h) == filepath.Clean(uh) {
		return "~"
	}
	return h
}

// passwdHome returns the current user's home directory straight from the OS
// user database, deliberately bypassing os.UserHomeDir because the latter
// prefers the $HOME environment variable — which is exactly the value callers
// of HomeDisplay are trying to cross-check against. Returns "" on any error.
func passwdHome() string {
	u, err := user.Current()
	if err != nil || u.HomeDir == "" {
		return ""
	}
	return u.HomeDir
}

// DisplayPath renders an absolute path for humans/UIs by replacing the home
// prefix with HomeDisplay(). Used by `hlops bridge paths` and doc snippets so
// the Companion APK and CLI always show the same stable file locations.
func DisplayPath(p string) string {
	h := Home()
	if h == "" || p == "" {
		return p
	}
	p = filepath.Clean(p)
	h = filepath.Clean(h)
	if p == h {
		return HomeDisplay()
	}
	if strings.HasPrefix(p, h+string(filepath.Separator)) {
		return HomeDisplay() + string(filepath.Separator) + strings.TrimPrefix(p, h+string(filepath.Separator))
	}
	return p
}

// SharedStorage returns ~/storage/shared if it exists (Termux after
// termux-setup-storage), otherwise empty string.
func SharedStorage() string { return termux.Detect().SharedStorage }

// TermuxHome is the well-known Termux $HOME for regular builds. The Companion
// APK runs outside the Termux sandbox and cannot query $HOME, so it derives
// hlops file locations from this constant plus the app-private variants below.
const TermuxHome = "/data/data/com.termux/files/home"

// TermuxHomeVariants returns every plausible Termux $HOME layout, most-likely
// first: the regular build, then common alternative package IDs (debug build,
// the long-deprecated Google Play build). Older devices with case-sensitive
// ext4 may also store app data under a capitalized directory name.
func TermuxHomeVariants() []string {
	return []string{
		TermuxHome,
		"/data/data/com.termux.debug/files/home",
		"/data/data/com.termux.legacy/files/home",
		"/data/data/com.Termux/files/home",
	}
}

// HomeRel joins sub-directions under a home dir without depending on the
// runtime OS separator (the APK resolves *Android* device layouts even when
// unit tests run on other platforms).
func HomeRel(home string, rel ...string) string {
	return filepath.Join(append([]string{home}, rel...)...)
}

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
