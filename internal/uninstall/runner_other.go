//go:build !windows

package uninstall

import (
	"context"
	"errors"
)

// defaultUninstallerRunner is a non-Windows stub. Execute refuses mutation
// on non-Windows before the runner is reached, so this type exists only to
// keep the package compiling on non-Windows without introducing a real
// shell command interpreter.
type defaultUninstallerRunner struct{}

func (defaultUninstallerRunner) Run(context.Context, string) (UninstallerRunResult, error) {
	return UninstallerRunResult{}, errors.New("uninstaller execution is Windows-only")
}
