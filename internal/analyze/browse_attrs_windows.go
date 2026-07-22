//go:build windows

package analyze

import (
	"os"
	"syscall"
)

func platformPresentationAttributes(path string, info os.FileInfo) presentationAttributes {
	// Prefer Sys() attributes when available (from Lstat); fall back to GetFileAttributes.
	if info != nil {
		if data, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
			return presentationAttributes{
				Hidden:  data.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0,
				System:  data.FileAttributes&syscall.FILE_ATTRIBUTE_SYSTEM != 0,
				Reparse: data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0,
			}
		}
	}
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return presentationAttributes{}
	}
	attrs, err := syscall.GetFileAttributes(ptr)
	if err != nil {
		return presentationAttributes{}
	}
	return presentationAttributes{
		Hidden:  attrs&syscall.FILE_ATTRIBUTE_HIDDEN != 0,
		System:  attrs&syscall.FILE_ATTRIBUTE_SYSTEM != 0,
		Reparse: attrs&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0,
	}
}
