//go:build !windows

package pathsafe

// isSupportedAnalyzeVolume is a non-Windows stub. Foal Analyze is Windows-native;
// non-Windows builds reject all volume roots fail-closed.
func isSupportedAnalyzeVolume(volumeRoot string) bool {
	_ = volumeRoot
	return false
}
