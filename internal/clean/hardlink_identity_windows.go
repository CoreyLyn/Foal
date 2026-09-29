//go:build windows

package clean

import (
	"os"
	"syscall"
)

type hardlinkIdentity struct {
	volume uint64
	file   uint64
}

func fileHardlinkIdentity(path string, _ os.FileInfo) (hardlinkIdentity, uint64, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return hardlinkIdentity{}, 0, err
	}
	handle, err := syscall.CreateFile(name, 0,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return hardlinkIdentity{}, 0, err
	}
	defer syscall.CloseHandle(handle)
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(handle, &info); err != nil {
		return hardlinkIdentity{}, 0, err
	}
	return hardlinkIdentity{
		volume: uint64(info.VolumeSerialNumber),
		file:   uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow),
	}, uint64(info.NumberOfLinks), nil
}
