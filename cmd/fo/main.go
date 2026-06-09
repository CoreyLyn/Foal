package main

import (
	"os"
	"path/filepath"

	"github.com/CoreyLyn/Foal/internal/cli"
)

func main() {
	os.Exit(cli.RunInvocation(cli.Invocation{
		ExecutableName:      filepath.Base(os.Args[0]),
		Args:                os.Args[1:],
		InteractiveTerminal: isInteractiveTerminal(os.Stdin),
		Input:               os.Stdin,
	}, os.Stdout, os.Stderr))
}

func isInteractiveTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
