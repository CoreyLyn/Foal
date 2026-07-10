package clean

// RecycleBinVolumeCapacity returns the Recycle Bin configuration for the
// volume containing the given path. Unsupported or unknown state returns an
// error so confirmed execution can fail closed.
func RecycleBinVolumeCapacity(path string) (RecycleBinVolumeConfig, error) {
	return recycleBinVolumeCapacity(path)
}
