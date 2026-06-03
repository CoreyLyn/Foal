//go:build windows

package delete

import (
	"errors"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	foDelete          = 0x0003
	fofAllowUndo      = 0x0040
	fofNoConfirmation = 0x0010
	fofNoErrorUI      = 0x0400
	fofSilent         = 0x0004
)

var (
	shell32              = syscall.NewLazyDLL("shell32.dll")
	procSHFileOperationW = shell32.NewProc("SHFileOperationW")
)

type WindowsRecycleBinAdapter struct{}

func (WindowsRecycleBinAdapter) MoveToRecycleBin(path string) error {
	from := utf16.Encode([]rune(path + "\x00\x00"))

	op := shFileOpStruct{
		wFunc: foDelete,
		pFrom: &from[0],
		fFlags: fofAllowUndo |
			fofNoConfirmation |
			fofNoErrorUI |
			fofSilent,
	}

	result, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if result != 0 {
		return syscall.Errno(result)
	}
	if op.fAnyOperationsAborted != 0 {
		return errors.New("Recycle Bin operation was aborted")
	}
	return nil
}

type shFileOpStruct struct {
	hWnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}
