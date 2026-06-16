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
