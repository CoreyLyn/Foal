//go:build !windows

package clean

import "context"

// productionDetectWindowsUpdateServices always reports unknown on non-Windows,
// which skips the entire category. The read-only SCM query is Windows-only.
func productionDetectWindowsUpdateServices(ctx context.Context) WindowsUpdateServicesState {
	return WindowsUpdateServicesState{Status: WindowsUpdateServicesUnknown, Message: "Windows Update download cache is only supported on Windows"}
}
