package main

import (
	"os"
	"path/filepath"

	"github.com/CoreyLyn/Foal/internal/cli"
)

func main() {
	os.Exit(cli.RunInvocation(cli.Invocation{
		ExecutableName:            filepath.Base(os.Args[0]),
		Args:                      os.Args[1:],
		InteractiveTerminal:       cli.IsInteractiveTerminal(os.Stdin),
		OutputInteractiveTerminal: cli.IsInteractiveTerminal(os.Stdout),
		Input:                     os.Stdin,
	}, os.Stdout, os.Stderr))
}
