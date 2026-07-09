package clean

// RecycleBinVolumeCapacity returns the Recycle Bin configuration for the
// volume containing the given path. It uses the real Windows registry on
// Windows, and returns a safe default (not disabled, large capacity) on
// other platforms.
func RecycleBinVolumeCapacity(path string) (RecycleBinVolumeConfig, error) {
	return recycleBinVolumeCapacity(path)
}
