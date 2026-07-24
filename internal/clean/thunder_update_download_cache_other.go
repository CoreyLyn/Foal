//go:build !windows

package clean

import "context"

// productionDetectThunderUpdateDownloadActivity always reports unknown on non-Windows, which
// skips the entire category.
func productionDetectThunderUpdateDownloadActivity(ctx context.Context) FixedRootActivityState {
	return FixedRootActivityState{Status: FixedRootActivityUnknown, Message: "Thunder update download cache is only supported on Windows"}
}
