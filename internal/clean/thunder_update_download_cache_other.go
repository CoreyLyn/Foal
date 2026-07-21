//go:build !windows

package clean

import "context"

// productionDetectThunderUpdateDownloadActivity always reports unknown on non-Windows, which
// skips the entire category.
func productionDetectThunderUpdateDownloadActivity(ctx context.Context) ThunderUpdateDownloadActivityState {
	return ThunderUpdateDownloadActivityState{Status: ThunderUpdateDownloadActivityUnknown, Message: "Thunder update download cache is only supported on Windows"}
}
