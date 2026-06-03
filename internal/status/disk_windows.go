//go:build windows

package status

import (
	"syscall"
	"unsafe"
)

var getDiskFreeSpaceEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

func captureDisk(path string) (DiskSnapshot, error) {
	var availableBytes, totalBytes, freeBytes uint64
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return DiskSnapshot{Path: path}, err
	}
	ret, _, err := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&availableBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&freeBytes)),
	)
	if ret == 0 {
		return DiskSnapshot{Path: path}, err
	}
	return DiskSnapshot{
		Path:           path,
		TotalBytes:     totalBytes,
		FreeBytes:      freeBytes,
		AvailableBytes: availableBytes,
	}, nil
}
