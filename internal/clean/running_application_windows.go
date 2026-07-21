//go:build windows

package clean

import (
	"context"
	"unsafe"

	"golang.org/x/sys/windows"
)

func snapshotProcesses(ctx context.Context) processSnapshot {
	select {
	case <-ctx.Done():
		return processSnapshot{Err: ctx.Err()}
	default:
	}

	handle, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return processSnapshot{Err: err}
	}
	defer windows.CloseHandle(handle)

	entry := windows.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	err = windows.Process32First(handle, &entry)
	if err != nil {
		return processSnapshot{Err: err}
	}

	names := []string{}
	for {
		select {
		case <-ctx.Done():
			return processSnapshot{Err: ctx.Err()}
		default:
		}
		names = append(names, windows.UTF16ToString(entry.ExeFile[:]))
		err = windows.Process32Next(handle, &entry)
		if err != nil {
			if err == windows.ERROR_NO_MORE_FILES {
				return processSnapshot{Names: names}
			}
			return processSnapshot{Err: err}
		}
	}
}

// snapshotServices returns a read-only snapshot of non-stopped Windows service names.
// It opens the SCM with read-only enumeration access; any failure is returned as an
// error so the caller fails closed to unknown.
func snapshotServices(ctx context.Context) serviceSnapshot {
	select {
	case <-ctx.Done():
		return serviceSnapshot{Err: ctx.Err()}
	default:
	}

	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT|windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return serviceSnapshot{Err: err}
	}
	defer windows.CloseServiceHandle(scm)

	var bytesNeeded, servicesReturned, resumeHandle uint32
	err = windows.EnumServicesStatusEx(scm, windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_WIN32, windows.SERVICE_STATE_ALL, nil, 0, &bytesNeeded, &servicesReturned, &resumeHandle, nil)
	if err != nil && err != windows.ERROR_MORE_DATA {
		return serviceSnapshot{Err: err}
	}
	if bytesNeeded == 0 {
		return serviceSnapshot{}
	}
	buf := make([]byte, bytesNeeded)
	err = windows.EnumServicesStatusEx(scm, windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_WIN32, windows.SERVICE_STATE_ALL, &buf[0], uint32(len(buf)), &bytesNeeded, &servicesReturned, &resumeHandle, nil)
	if err != nil {
		return serviceSnapshot{Err: err}
	}
	if servicesReturned == 0 {
		return serviceSnapshot{}
	}
	services := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0])), int(servicesReturned))
	nonStopped := []string{}
	for i := range services {
		select {
		case <-ctx.Done():
			return serviceSnapshot{Err: ctx.Err()}
		default:
		}
		service := services[i]
		if service.ServiceStatusProcess.CurrentState != windows.SERVICE_STOPPED {
			var name string
			if service.ServiceName != nil {
				name = windows.UTF16PtrToString(service.ServiceName)
			}
			if name != "" {
				nonStopped = append(nonStopped, name)
			}
		}
	}
	return serviceSnapshot{NonStopped: nonStopped}
}
