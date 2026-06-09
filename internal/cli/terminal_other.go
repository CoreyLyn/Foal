//go:build !windows

package cli

import "os"

func IsInteractiveTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func enableRawInputFile(file *os.File) func() {
	return func() {}
}

func enableVirtualTerminalOutputFile(file *os.File) func() {
	return func() {}
}
