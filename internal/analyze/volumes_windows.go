//go:build windows

package analyze

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// platformDriveProbe implements DriveProbe with Win32 volume APIs only.
// It does not open directory handles for enumeration or walk children.
type platformDriveProbe struct{}

var getDiskFreeSpaceExW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

func (platformDriveProbe) LogicalDriveMask() (uint32, error) {
	return windows.GetLogicalDrives()
}

func (platformDriveProbe) SupportedKind(root string) (VolumeKind, bool) {
	ptr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return "", false
	}
	switch windows.GetDriveType(ptr) {
	case windows.DRIVE_FIXED:
		return VolumeKindFixed, true
	case windows.DRIVE_REMOVABLE:
		return VolumeKindRemovable, true
	default:
		return "", false
	}
}

func (platformDriveProbe) ProbeMetadata(root string) VolumeMetadata {
	meta := VolumeMetadata{}

	rootPtr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return meta
	}

	var (
		volumeName [windows.MAX_PATH + 1]uint16
		fsName     [windows.MAX_PATH + 1]uint16
		serial     uint32
		maxComp    uint32
		flags      uint32
	)
	infoErr := windows.GetVolumeInformation(
		rootPtr,
		&volumeName[0],
		uint32(len(volumeName)),
		&serial,
		&maxComp,
		&flags,
		&fsName[0],
		uint32(len(fsName)),
	)
	if infoErr == nil {
		meta.Label = windows.UTF16ToString(volumeName[:])
		meta.FileSystem = windows.UTF16ToString(fsName[:])
	}

	var availableBytes, totalBytes, freeBytes uint64
	ret, _, _ := getDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(rootPtr)),
		uintptr(unsafe.Pointer(&availableBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&freeBytes)),
	)
	if ret != 0 {
		meta.Available = true
		meta.HasCapacity = true
		meta.TotalBytes = totalBytes
		meta.FreeBytes = freeBytes
		return meta
	}

	// Capacity failed: volume may still be present (empty media, locked).
	// Treat as unavailable so drive entry can list it without allowing enter.
	meta.Available = false
	meta.HasCapacity = false
	return meta
}
