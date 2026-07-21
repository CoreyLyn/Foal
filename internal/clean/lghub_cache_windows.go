//go:build windows

package clean

import (
	"context"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// productionDetectLGHUBActivity conservatively reports LG HUB process/service
// activity. Any lghub-named process or non-stopped LG HUB-owned service is
// running; any enumeration failure or uncertain attribution is unknown. Idle
// requires both a clean process snapshot and a reachable, all-idle LG HUB
// service set. Clean never stops these processes or services.
func productionDetectLGHUBActivity(ctx context.Context) LGHUBActivityState {
	snapshot := snapshotProcesses(ctx)
	if snapshot.Err != nil {
		return LGHUBActivityState{Status: LGHUBActivityUnknown, Message: "LG HUB process snapshot was unavailable"}
	}
	for _, name := range snapshot.Names {
		if isLGHUBProcessName(name) {
			return LGHUBActivityState{Status: LGHUBActivityRunning, Message: "an LG HUB process is running"}
		}
	}
	active, err := lghubServicesActive()
	if err != nil {
		return LGHUBActivityState{Status: LGHUBActivityUnknown, Message: "LG HUB service state could not be determined"}
	}
	if active {
		return LGHUBActivityState{Status: LGHUBActivityRunning, Message: "an LG HUB service is running"}
	}
	return LGHUBActivityState{Status: LGHUBActivityIdle}
}

// isLGHUBProcessName matches LG HUB process names. This is intentionally broad
// and accepts frequent false skips over risking an unlisted writer.
func isLGHUBProcessName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	return strings.HasPrefix(lower, "lghub") || strings.Contains(lower, "lghub") ||
		strings.HasPrefix(lower, "lg hub") || strings.Contains(lower, "lg hub")
}

// isLGHUBServiceName matches LG HUB-owned service short or display names using
// the same broad lghub heuristic.
func isLGHUBServiceName(serviceName, displayName string) bool {
	return isLGHUBProcessName(serviceName) || isLGHUBProcessName(displayName)
}

// lghubServicesActive reports whether any LG HUB-owned Win32 service is in a
// non-stopped state. It opens the SCM with read-only enumeration access; any
// failure is returned as an error so the caller fails closed to unknown.
func lghubServicesActive() (bool, error) {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT|windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return false, err
	}
	defer windows.CloseServiceHandle(scm)

	var bytesNeeded, servicesReturned, resumeHandle uint32
	err = windows.EnumServicesStatusEx(scm, windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_WIN32, windows.SERVICE_STATE_ALL, nil, 0, &bytesNeeded, &servicesReturned, &resumeHandle, nil)
	if err != nil && err != windows.ERROR_MORE_DATA {
		return false, err
	}
	if bytesNeeded == 0 {
		return false, nil
	}
	buf := make([]byte, bytesNeeded)
	err = windows.EnumServicesStatusEx(scm, windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_WIN32, windows.SERVICE_STATE_ALL, &buf[0], uint32(len(buf)), &bytesNeeded, &servicesReturned, &resumeHandle, nil)
	if err != nil {
		return false, err
	}
	if servicesReturned == 0 {
		return false, nil
	}
	services := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0])), int(servicesReturned))
	for i := range services {
		service := services[i]
		name := utf16PtrToStringSafe(service.ServiceName)
		display := utf16PtrToStringSafe(service.DisplayName)
		if !isLGHUBServiceName(name, display) {
			continue
		}
		if service.ServiceStatusProcess.CurrentState != windows.SERVICE_STOPPED {
			return true, nil
		}
	}
	return false, nil
}
