//go:build !windows

package clean

// DefaultRecycleBinMaxCapacity is a large default capacity for non-Windows.
const DefaultRecycleBinMaxCapacity = 100 * 1024 * 1024 * 1024 // 100 GB

func recycleBinVolumeCapacity(path string) (RecycleBinVolumeConfig, error) {
	// On non-Windows platforms, default to Recycle Bin enabled with large capacity
	return RecycleBinVolumeConfig{
		NukeOnDelete: false,
		MaxCapacity:  DefaultRecycleBinMaxCapacity,
	}, nil
}
