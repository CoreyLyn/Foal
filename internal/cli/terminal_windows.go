//go:build windows

package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

func IsInteractiveTerminal(file *os.File) bool {
	if file == nil {
		return false
	}

	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(file.Fd()), &mode); err == nil {
		return true
	}

	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func enableRawInputFile(file *os.File) func() {
	if file == nil {
		return func() {}
	}

	handle := windows.Handle(file.Fd())
	var originalMode uint32
	if err := windows.GetConsoleMode(handle, &originalMode); err != nil {
		return func() {}
	}

	rawMode := (originalMode &^ (windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT)) |
		windows.ENABLE_PROCESSED_INPUT |
		windows.ENABLE_VIRTUAL_TERMINAL_INPUT
	if err := windows.SetConsoleMode(handle, rawMode); err != nil {
		rawMode = (originalMode &^ (windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT)) |
			windows.ENABLE_PROCESSED_INPUT
		if err := windows.SetConsoleMode(handle, rawMode); err != nil {
			return func() {}
		}
	}

	return func() {
		_ = windows.SetConsoleMode(handle, originalMode)
	}
}

func enableVirtualTerminalOutputFile(file *os.File) func() {
	if file == nil {
		return func() {}
	}

	handle := windows.Handle(file.Fd())
	var originalMode uint32
	if err := windows.GetConsoleMode(handle, &originalMode); err != nil {
		return func() {}
	}

	mode := originalMode | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	if err := windows.SetConsoleMode(handle, mode); err != nil {
		return func() {}
	}

	return func() {
		_ = windows.SetConsoleMode(handle, originalMode)
	}
}
