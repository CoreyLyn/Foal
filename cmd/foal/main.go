package main

import (
	"os"
	"path/filepath"

	"github.com/CoreyLyn/Foal/internal/cli"
	"github.com/CoreyLyn/Foal/internal/servicing"
)

func main() {
	// The internal, elevated servicing helper mode is routed before any ordinary
	// CLI parsing or TUI so it is never reachable as a user command. It acts only
	// after authenticating its launching coordinator over the nonce-bound pipe.
	if servicing.IsHelperInvocation(os.Args[1:]) {
		os.Exit(servicing.RunHelper(os.Args[1:]))
	}
	os.Exit(cli.RunInvocation(cli.Invocation{
		ExecutableName:            filepath.Base(os.Args[0]),
		Args:                      os.Args[1:],
		InteractiveTerminal:       cli.IsInteractiveTerminal(os.Stdin),
		OutputInteractiveTerminal: cli.IsInteractiveTerminal(os.Stdout),
		Input:                     os.Stdin,
	}, os.Stdout, os.Stderr))
}
