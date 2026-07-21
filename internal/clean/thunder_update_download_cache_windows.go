//go:build windows

package clean

import (
	"context"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// productionDetectThunderUpdateDownloadActivity conservatively reports Thunder
// process/service activity. Any Thunder-named process or non-stopped Thunder-owned
// service is running; any enumeration failure or uncertain attribution is unknown.
// Idle requires both a clean process snapshot and a reachable, all-idle Thunder
// service set. Clean never stops these processes or services.
func productionDetectThunderUpdateDownloadActivity(ctx context.Context) ThunderUpdateDownloadActivityState {
	snapshot := snapshotProcesses(ctx)
	if snapshot.Err != nil {
		return ThunderUpdateDownloadActivityState{Status: ThunderUpdateDownloadActivityUnknown, Message: "Thunder process snapshot was unavailable"}
	}
	for _, name := range snapshot.Names {
		if isThunderProcessName(name) {
			return ThunderUpdateDownloadActivityState{Status: ThunderUpdateDownloadActivityRunning, Message: "a Thunder process is running"}
		}
	}
	active, err := thunderServicesActive()
	if err != nil {
		return ThunderUpdateDownloadActivityState{Status: ThunderUpdateDownloadActivityUnknown, Message: "Thunder service state could not be determined"}
	}
	if active {
		return ThunderUpdateDownloadActivityState{Status: ThunderUpdateDownloadActivityRunning, Message: "a Thunder service is running"}
	}
	return ThunderUpdateDownloadActivityState{Status: ThunderUpdateDownloadActivityIdle}
}

// isThunderProcessName matches Thunder process names. This is intentionally broad
// and accepts frequent false skips over risking an unlisted writer.
func isThunderProcessName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "thunder") || strings.Contains(lower, "xlliveud")
}

// isThunderServiceName matches Thunder-owned service short or display names using
// the same broad Thunder heuristic.
func isThunderServiceName(serviceName, displayName string) bool {
	return isThunderProcessName(serviceName) || isThunderProcessName(displayName)
}

// thunderServicesActive reports whether any Thunder-owned Win32 service is in a
// non-stopped state. It opens the SCM with read-only enumeration access; any
// failure is returned as an error so the caller fails closed to unknown.
func thunderServicesActive() (bool, error) {
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
		if !isThunderServiceName(name, display) {
			continue
		}
		if service.ServiceStatusProcess.CurrentState != windows.SERVICE_STOPPED {
			return true, nil
		}
	}
	return false, nil
}
