//go:build !windows

package clean

import (
	"context"
	"errors"
)

func snapshotProcesses(context.Context) processSnapshot {
	return processSnapshot{Err: errors.New("Windows process snapshot is unavailable on this platform")}
}

func snapshotServices(context.Context) serviceSnapshot {
	return serviceSnapshot{Err: errors.New("Windows service snapshot is unavailable on this platform")}
}
