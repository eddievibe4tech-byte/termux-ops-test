// Package lock provides a single-instance advisory lock for hlops run.
// Lock files live under $HOME/.local/state/hlops/locks. A second instance
// gets ErrLocked which the CLI maps to exit code 6.
package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/eddievibe4tech-byte/termux-ops/internal/paths"
)

// ErrLocked is returned when another live process holds the lock.
var ErrLocked = errors.New("another hlops instance holds the lock")

// ExitCodeLocked is the process exit code when the lock cannot be acquired.
const ExitCodeLocked = 6

// Lock is an acquired advisory lock.
type Lock struct {
	path string
	f    *os.File
}

// Acquire takes an exclusive, non-blocking flock on <name>.lock in the lock
// dir. Stale locks are impossible because flock is released when the holding
// process dies.
func Acquire(name string) (*Lock, error) {
	dir := paths.LockDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	p := filepath.Join(dir, name+".lock")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		// include holder pid for diagnostics
		data, _ := os.ReadFile(p)
		f.Close()
		return nil, fmt.Errorf("%w: held by pid %s", ErrLocked, trim(string(data)))
	}
	if _, err := f.WriteAt([]byte(fmt.Sprintf("%d\n", os.Getpid())), 0); err == nil {
		_ = f.Truncate(16)
	}
	return &Lock{path: p, f: f}, nil
}

// Release unlocks and removes the lock file.
func (l *Lock) Release() {
	if l == nil || l.f == nil {
		return
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	_ = l.f.Close()
	_ = os.Remove(l.path)
	l.f = nil
}

func trim(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	if s == "" {
		return "?"
	}
	return s
}

// Age reports how old the named lock file is (diagnostics only).
func Age(name string) (time.Duration, error) {
	fi, err := os.Stat(filepath.Join(paths.LockDir(), name+".lock"))
	if err != nil {
		return 0, err
	}
	return time.Since(fi.ModTime()), nil
}
