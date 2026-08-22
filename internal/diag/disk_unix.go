//go:build unix

package diag

import "syscall"

// diskFree returns free bytes on the filesystem containing path. ok is false
// when the platform cannot report free space.
func diskFree(path string) (int64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, false
	}
	return int64(st.Bavail) * int64(st.Bsize), true
}
