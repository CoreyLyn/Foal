//go:build !windows

package clean

import "context"

// productionDetectLGHUBActivity always reports unknown on non-Windows, which
// skips the entire category.
func productionDetectLGHUBActivity(ctx context.Context) LGHUBActivityState {
	return LGHUBActivityState{Status: LGHUBActivityUnknown, Message: "LG HUB cache is only supported on Windows"}
}
