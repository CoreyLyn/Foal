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
