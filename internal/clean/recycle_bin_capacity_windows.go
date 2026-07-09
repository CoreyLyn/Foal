//go:build windows

package clean

import (
	"golang.org/x/sys/windows/registry"
)

// DefaultRecycleBinMaxCapacity is the default maximum Recycle Bin capacity
// in bytes when no explicit configuration is found. This uses the Windows
// default of ~5% of the drive (but represented as a large safe default here).
const DefaultRecycleBinMaxCapacity = 100 * 1024 * 1024 * 1024 // 100 GB

func recycleBinVolumeCapacity(path string) (RecycleBinVolumeConfig, error) {
	config := RecycleBinVolumeConfig{
		NukeOnDelete: false,
		MaxCapacity:  DefaultRecycleBinMaxCapacity,
	}

	// Check global BitBucket configuration
	return checkGlobalBitBucketConfig(config)
}

func checkGlobalBitBucketConfig(defaultConfig RecycleBinVolumeConfig) (RecycleBinVolumeConfig, error) {
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

	// Read MaxCapacity (DWORD: percentage if <=100, MB if >100)
	maxCapacityVal, _, err := key.GetIntegerValue("MaxCapacity")
	if err == nil {
		config.MaxCapacity = interpretMaxCapacity(maxCapacityVal)
	}

	return config, nil
}

func interpretMaxCapacity(val uint64) int64 {
	if val == 0 {
		return DefaultRecycleBinMaxCapacity
	}
	if val <= 100 {
		// It's a percentage - use our large default as a safe maximum
		// since we don't know the actual drive size
		return DefaultRecycleBinMaxCapacity
	}
	// It's megabytes
	return int64(val) * 1024 * 1024
}
