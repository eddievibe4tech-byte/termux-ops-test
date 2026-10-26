// Package termux provides detection of the Termux (Android) environment.
package termux

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// constTermuxPrefix is the well-known Termux private data prefix. It is used
// only as a fallback hint when environment variables are missing; behaviour
// is never hard-coded on it alone.
const constTermuxPrefix = "/data/data/com.termux/files/usr"

// Info describes the detected runtime environment.
type Info struct {
	IsTermux      bool
	Version       string // TERMUX_VERSION if set
	Prefix        string // $PREFIX (Termux) or derived
	Home          string // resolved home directory
	TmpDir        string // $TMPDIR or prefix/tmp fallback
	SharedStorage string // ~/storage/shared if available
}

var (
	cached     *Info
	detectOnce sync.Once
)

// Detect inspects the environment and returns a memoised Termux info.
// Detection rules:
//   - TERMUX_VERSION env var set => Termux.
//   - PREFIX equals the well-known Termux prefix => Termux.
//   - HOME under /data/data/com.termux/files/... => Termux.
//
// Plain Linux must not be misdetected. The result is computed exactly once
// via sync.Once so concurrent callers (future goroutine-based scanning)
// cannot race on the memoised value.
func Detect() *Info {
	detectOnce.Do(func() {
		home := os.Getenv("HOME")
		prefix := os.Getenv("PREFIX")
		version := os.Getenv("TERMUX_VERSION")

		isTermux := version != "" ||
			filepath.Clean(prefix) == constTermuxPrefix ||
			strings.HasPrefix(home, "/data/data/com.termux/files/")

		if isTermux && prefix == "" {
			prefix = constTermuxPrefix
		}
		if !isTermux {
			version = ""
		}

		tmpdir := os.Getenv("TMPDIR")
		if tmpdir == "" && isTermux {
			tmpdir = filepath.Join(prefix, "tmp")
		}

		i := &Info{
			IsTermux:      isTermux,
			Version:       version,
			Prefix:        prefix,
			Home:          home,
			TmpDir:        tmpdir,
			SharedStorage: sharedStoragePath(home),
		}
		cached = i
	})
	return cached
}

// ResetCache clears the memoised detection result (used by tests).
// It re-arms the sync.Once so the next Detect() re-reads the environment.
func ResetCache() {
	cached = nil
	detectOnce = sync.Once{}
}

func sharedStoragePath(home string) string {
	if home == "" {
		return ""
	}
	p := filepath.Join(home, "storage", "shared")
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return p
	}
	return ""
}
