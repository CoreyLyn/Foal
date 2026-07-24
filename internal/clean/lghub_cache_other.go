//go:build !windows

package clean

import "context"

// productionDetectLGHUBActivity always reports unknown on non-Windows, which
// skips the entire category.
func productionDetectLGHUBActivity(ctx context.Context) FixedRootActivityState {
	return FixedRootActivityState{Status: FixedRootActivityUnknown, Message: "LG HUB cache is only supported on Windows"}
}
