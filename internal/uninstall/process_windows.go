//go:build windows

package uninstall

import (
	"context"
	"strings"
	"unicode"
	"unsafe"

	"golang.org/x/sys/windows"
)

// defaultProcessDetector reports whether an application is currently running
// by snapshotting Windows processes via Toolhelp32 and matching the app's
// normalized display name against process executable names. The match is
// conservative: it normalizes both sides (lower-case, drop non-alphanumeric
// runes) and treats a process as matching when its normalized executable
// name equals the normalized app name OR the app name is a prefix of the
// process executable name (so "Chrome" matches "chrome.exe"). This catches
// obvious cases without false positives on unrelated apps. Unknown snapshot
// errors yield ProcessStateUnknown so Execute proceeds cautiously rather
// than skipping every app when detection is temporarily unavailable.
type defaultProcessDetector struct{}

func (defaultProcessDetector) IsRunning(ctx context.Context, appName string) (ProcessState, error) {
	select {
	case <-ctx.Done():
		return ProcessState{State: ProcessStateUnknown, Message: ctx.Err().Error()}, nil
	default:
	}

	handle, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ProcessState{State: ProcessStateUnknown, Message: err.Error()}, nil
	}
	defer windows.CloseHandle(handle)

	entry := windows.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(handle, &entry); err != nil {
		return ProcessState{State: ProcessStateUnknown, Message: err.Error()}, nil
	}

	target := normalizeProcessName(appName)
	if target == "" {
		return ProcessState{State: ProcessStateIdle}, nil
	}

	for {
		select {
		case <-ctx.Done():
			return ProcessState{State: ProcessStateUnknown, Message: ctx.Err().Error()}, nil
		default:
		}
		exeName := windows.UTF16ToString(entry.ExeFile[:])
		normalized := normalizeProcessName(exeName)
		if normalized == target || strings.HasPrefix(normalized, target) {
			return ProcessState{State: ProcessStateRunning, Message: exeName}, nil
		}
		if err := windows.Process32Next(handle, &entry); err != nil {
			if err == windows.ERROR_NO_MORE_FILES {
				break
			}
			return ProcessState{State: ProcessStateUnknown, Message: err.Error()}, nil
		}
	}
	return ProcessState{State: ProcessStateIdle}, nil
}

// normalizeProcessName lower-cases a name and drops every non-alphanumeric
// rune so "Google Chrome" -> "googlechrome" and "chrome.exe" -> "chromeexe".
// The prefix match then lets "Chrome" match "chromeexe" without matching
// unrelated process names.
func normalizeProcessName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
