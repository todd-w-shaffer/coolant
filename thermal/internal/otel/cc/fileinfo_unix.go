//go:build unix

package cc

import (
	"os"
	"syscall"
)

// inodeOf returns the inode of a stat result on Unix platforms.
// On non-unix builds, the tailer fallback is offset-only (truncation
// detection still works; recreation-with-same-size is rare on Windows).
func inodeOf(info os.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Ino)
	}
	return 0
}
