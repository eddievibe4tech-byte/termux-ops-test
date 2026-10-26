//go:build linux || darwin

package backup

import (
	"io/fs"
	"syscall"
)

// fileOwner extracts uid/gid from a FileInfo on Unix-like systems.
func fileOwner(fi fs.FileInfo) (uid, gid int) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid), int(st.Gid)
	}
	return -1, -1
}
