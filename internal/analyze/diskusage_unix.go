//go:build linux || darwin

package analyze

import "syscall"

// statfs reports free/total space for the mount point containing path.
// syscall.Statfs is available on Linux (Termux/Android) and Darwin;
// other platforms use the fallback in diskusage_other.go.
func statfs(path string) Volume {
	var st syscall.Statfs_t
	v := Volume{Path: path}
	if err := syscall.Statfs(path, &st); err != nil {
		v.Error = err.Error()
		return v
	}
	bsize := uint64(st.Bsize)
	v.TotalBytes = uint64(st.Blocks) * bsize
	v.FreeBytes = uint64(st.Bfree) * bsize
	v.Available = uint64(st.Bavail) * bsize
	return v
}
