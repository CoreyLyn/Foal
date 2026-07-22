//go:build windows

package clean

import (
	"context"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// productionDetectWindowsUpdateServices conservatively reports Windows Update
// service-stack state via a read-only SCM enumeration. Any of the exact update
// services (wuauserv, bits, dosvc, UsoSvc) in a non-Stopped state is running;
// any enumeration failure is unknown. A service that is absent or Stopped does
// not block. Foal only observes and never starts, stops, or reconfigures any
// service.
func productionDetectWindowsUpdateServices(ctx context.Context) WindowsUpdateServicesState {
	select {
	case <-ctx.Done():
		return WindowsUpdateServicesState{Status: WindowsUpdateServicesUnknown, Message: "Windows Update service query was canceled"}
	default:
	}

	active, err := windowsUpdateServicesActive(ctx)
	if err != nil {
		return WindowsUpdateServicesState{Status: WindowsUpdateServicesUnknown, Message: "Windows Update service state could not be determined"}
	}
	if active {
		return WindowsUpdateServicesState{Status: WindowsUpdateServicesRunning, Message: "a Windows Update service is active"}
	}
	return WindowsUpdateServicesState{Status: WindowsUpdateServicesIdle}
}

// isWindowsUpdateServiceName reports whether serviceName exactly matches one of
// the declared Windows Update service short names (case-insensitive).
func isWindowsUpdateServiceName(serviceName string) bool {
	name := strings.TrimSpace(serviceName)
	if name == "" {
		return false
	}
	for _, want := range windowsUpdateServiceNames {
		if strings.EqualFold(name, want) {
			return true
		}
	}
	return false
}

// windowsUpdateServicesActive reports whether any of the exact Windows Update
// services is in a non-stopped state. It opens the SCM with read-only
// enumeration access; any failure is returned as an error so the caller fails
// closed to unknown.
func windowsUpdateServicesActive(ctx context.Context) (bool, error) {
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
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}
		service := services[i]
		name := utf16PtrToStringSafe(service.ServiceName)
		if !isWindowsUpdateServiceName(name) {
			continue
		}
		if service.ServiceStatusProcess.CurrentState != windows.SERVICE_STOPPED {
			return true, nil
		}
	}
	return false, nil
}
