//go:build windows

package clean

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	kernel32                        = windows.NewLazySystemDLL("kernel32.dll")
	shell32                         = windows.NewLazySystemDLL("shell32.dll")
	procGetVolumePathNameW          = kernel32.NewProc("GetVolumePathNameW")
	procGetVolumeNameForMountPointW = kernel32.NewProc("GetVolumeNameForVolumeMountPointW")
	procSHQueryRecycleBinW          = shell32.NewProc("SHQueryRecycleBinW")
)

type recycleBinVolumeProbeDependencies struct {
	volumeIdentity func(path string) (root string, volume string, err error)
	readConfig     func(volume string) (nukeOnDelete bool, maxCapacityValue uint64, err error)
	currentUsage   func(root string) (int64, error)
}

func recycleBinVolumeCapacity(path string) (RecycleBinVolumeConfig, error) {
	return recycleBinVolumeCapacityWithDependencies(path, recycleBinVolumeProbeDependencies{
		volumeIdentity: windowsVolumeIdentity,
		readConfig:     readRecycleBinVolumeConfig,
		currentUsage:   queryRecycleBinUsage,
	})
}

func recycleBinVolumeCapacityWithDependencies(path string, deps recycleBinVolumeProbeDependencies) (RecycleBinVolumeConfig, error) {
	root, volume, err := deps.volumeIdentity(path)
	config := RecycleBinVolumeConfig{Volume: volume}
	if err != nil {
		return config, fmt.Errorf("resolve Recycle Bin volume: %w", err)
	}
	if strings.TrimSpace(root) == "" || strings.TrimSpace(volume) == "" {
		return config, errors.New("resolve Recycle Bin volume: empty volume identity")
	}

	nukeOnDelete, maxCapacityValue, err := deps.readConfig(volume)
	if err != nil {
		return config, fmt.Errorf("read Recycle Bin configuration: %w", err)
	}
	config.NukeOnDelete = nukeOnDelete
	config.MaxCapacity, err = interpretMaxCapacity(maxCapacityValue)
	if err != nil {
		return config, fmt.Errorf("interpret Recycle Bin capacity: %w", err)
	}
	config.CurrentUsage, err = deps.currentUsage(root)
	if err != nil {
		return config, fmt.Errorf("query Recycle Bin usage: %w", err)
	}
	if config.CurrentUsage < 0 {
		return config, errors.New("query Recycle Bin usage: negative usage")
	}
	return config, nil
}

func windowsVolumeIdentity(path string) (string, string, error) {
	pathPtr, err := windows.UTF16PtrFromString(filepath.Clean(path))
	if err != nil {
		return "", "", err
	}
	rootBuffer := make([]uint16, windows.MAX_PATH+1)
	result, _, callErr := procGetVolumePathNameW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&rootBuffer[0])),
		uintptr(len(rootBuffer)),
	)
	if result == 0 {
		return "", "", windowsCallError("GetVolumePathNameW", callErr)
	}
	root := windows.UTF16ToString(rootBuffer)
	rootPtr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return "", "", err
	}
	volumeBuffer := make([]uint16, windows.MAX_PATH+1)
	result, _, callErr = procGetVolumeNameForMountPointW.Call(
		uintptr(unsafe.Pointer(rootPtr)),
		uintptr(unsafe.Pointer(&volumeBuffer[0])),
		uintptr(len(volumeBuffer)),
	)
	if result == 0 {
		return root, "", windowsCallError("GetVolumeNameForVolumeMountPointW", callErr)
	}
	return root, windows.UTF16ToString(volumeBuffer), nil
}

func windowsCallError(name string, err error) error {
	if err == nil || errors.Is(err, syscall.Errno(0)) {
		return errors.New(name + " failed")
	}
	return fmt.Errorf("%s: %w", name, err)
}

func readRecycleBinVolumeConfig(volume string) (bool, uint64, error) {
	volumeKey, err := recycleBinRegistryVolumeKey(volume)
	if err != nil {
		return false, 0, err
	}
	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Explorer\BitBucket\Volume\`+volumeKey,
		registry.READ,
	)
	if err != nil {
		return false, 0, err
	}
	defer key.Close()

	nukeOnDelete, _, err := key.GetIntegerValue("NukeOnDelete")
	if err != nil {
		return false, 0, err
	}
	maxCapacity, _, err := key.GetIntegerValue("MaxCapacity")
	if err != nil {
		return false, 0, err
	}
	return nukeOnDelete != 0, maxCapacity, nil
}

func recycleBinRegistryVolumeKey(volume string) (string, error) {
	trimmed := strings.TrimSuffix(strings.TrimSpace(volume), `\`)
	const prefix = `\\?\Volume`
	if !strings.HasPrefix(strings.ToLower(trimmed), strings.ToLower(prefix)) {
		return "", fmt.Errorf("unsupported volume identity %q", volume)
	}
	key := trimmed[len(prefix):]
	if key == "" {
		return "", fmt.Errorf("unsupported volume identity %q", volume)
	}
	return key, nil
}

func interpretMaxCapacity(value uint64) (int64, error) {
	if value > math.MaxInt64/(1024*1024) {
		return 0, errors.New("Recycle Bin capacity exceeds supported range")
	}
	return int64(value) * 1024 * 1024, nil
}

type shQueryRecycleBinInfo struct {
	CBSize      uint32
	I64Size     int64
	I64NumItems int64
}

func queryRecycleBinUsage(root string) (int64, error) {
	rootPtr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return 0, err
	}
	info := shQueryRecycleBinInfo{CBSize: uint32(unsafe.Sizeof(shQueryRecycleBinInfo{}))}
	hResult, _, _ := procSHQueryRecycleBinW.Call(
		uintptr(unsafe.Pointer(rootPtr)),
		uintptr(unsafe.Pointer(&info)),
	)
	if hResult != 0 {
		return 0, fmt.Errorf("SHQueryRecycleBinW failed with HRESULT 0x%08x", uint32(hResult))
	}
	return info.I64Size, nil
}
