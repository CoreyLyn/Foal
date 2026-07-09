//go:build windows

package clean

import (
	"path/filepath"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// DefaultRecycleBinMaxCapacity is the default maximum Recycle Bin capacity
// in bytes when no explicit configuration is found.
const DefaultRecycleBinMaxCapacity = 100 * 1024 * 1024 * 1024 // 100 GB

// volumeSizeFunc is the type for looking up a volume's total size in bytes.
type volumeSizeFunc func(path string) (int64, error)

// getVolumeSize is the real implementation that calls Windows GetDiskFreeSpaceExW.
func getVolumeSize(path string) (int64, error) {
	// Get the volume root (e.g., "C:\") from the path.
	volumeRoot := filepath.VolumeName(path) + "\\"

	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64
	volumeRootUTF16, err := windows.UTF16PtrFromString(volumeRoot)
	if err != nil {
		return DefaultRecycleBinMaxCapacity, err
	}

	err = windows.GetDiskFreeSpaceEx(volumeRootUTF16, &freeBytesAvailable, &totalNumberOfBytes, &totalNumberOfFreeBytes)
	if err != nil {
		return DefaultRecycleBinMaxCapacity, err
	}

	return int64(totalNumberOfBytes), nil
}

func recycleBinVolumeCapacity(path string) (RecycleBinVolumeConfig, error) {
	return recycleBinVolumeCapacityWithVolumeSize(path, getVolumeSize)
}

// recycleBinVolumeCapacityWithVolumeSize implements the probe logic with injectable volume size lookup for testing.
func recycleBinVolumeCapacityWithVolumeSize(path string, getVolumeSize volumeSizeFunc) (RecycleBinVolumeConfig, error) {
	config := RecycleBinVolumeConfig{
		NukeOnDelete: false,
		MaxCapacity:  DefaultRecycleBinMaxCapacity,
	}

	// Check global BitBucket configuration
	return checkGlobalBitBucketConfig(path, config, getVolumeSize)
}

func checkGlobalBitBucketConfig(path string, defaultConfig RecycleBinVolumeConfig, getVolumeSize volumeSizeFunc) (RecycleBinVolumeConfig, error) {
	config := defaultConfig

	// Open HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\BitBucket
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\BitBucket`, registry.READ)
	if err != nil {
		// If key doesn't exist, use defaults
		return config, nil
	}
	defer key.Close()

	// Read NukeOnDelete (DWORD: 1 = disabled)
	nukeOnDelete, _, err := key.GetIntegerValue("NukeOnDelete")
	if err == nil {
		config.NukeOnDelete = nukeOnDelete != 0
	}

	// Read MaxCapacity (DWORD: percentage 0-100 if <=100, MB if >100)
	maxCapacityVal, _, err := key.GetIntegerValue("MaxCapacity")
	if err == nil {
		config.MaxCapacity = interpretMaxCapacity(maxCapacityVal, path, getVolumeSize)
	}

	return config, nil
}

func interpretMaxCapacity(val uint64, path string, getVolumeSize volumeSizeFunc) int64 {
	if val == 0 {
		return DefaultRecycleBinMaxCapacity
	}
	if val <= 100 {
		// It's a percentage - calculate actual capacity from volume size
		volumeSize, err := getVolumeSize(path)
		if err != nil {
			return DefaultRecycleBinMaxCapacity
		}
		// Calculate percentage: (val / 100) * volumeSize
		return int64((uint64(volumeSize) * val) / 100)
	}
	// It's megabytes
	return int64(val) * 1024 * 1024
}
