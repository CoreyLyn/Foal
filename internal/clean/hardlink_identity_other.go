//go:build !windows

package clean

import (
	"os"
	"syscall"
)

type hardlinkIdentity struct {
	volume uint64
	file   uint64
}

func fileHardlinkIdentity(_ string, info os.FileInfo) (hardlinkIdentity, uint64, error) {
	stat := info.Sys().(*syscall.Stat_t)
	return hardlinkIdentity{volume: uint64(stat.Dev), file: uint64(stat.Ino)}, uint64(stat.Nlink), nil
}
