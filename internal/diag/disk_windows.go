//go:build windows

package diag

// diskFree is unimplemented on Windows (no statfs); the caller reports
// "free space unknown" instead of a hard failure.
func diskFree(string) (int64, bool) { return 0, false }
