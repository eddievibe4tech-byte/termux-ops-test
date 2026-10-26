//go:build !linux && !darwin

package backup

import "io/fs"

// fileOwner is a fallback for platforms without syscall.Stat_t. hlops does
// not officially support these platforms; the manifest simply records
// unknown ownership instead of failing the build or the backup.
func fileOwner(fi fs.FileInfo) (uid, gid int) { return -1, -1 }
