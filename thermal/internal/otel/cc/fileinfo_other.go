//go:build !unix

package cc

import "os"

func inodeOf(_ os.FileInfo) uint64 { return 0 }
