//go:build !windows

package uninstall

import "context"

// defaultProcessDetector is a non-Windows stub. Execute refuses mutation on
// non-Windows before the detector is reached, so this type reports idle to
// keep the package compiling. It must not be used to make real safety
// decisions on Windows.
type defaultProcessDetector struct{}

func (defaultProcessDetector) IsRunning(context.Context, string) (ProcessState, error) {
	return ProcessState{State: ProcessStateIdle}, nil
}
