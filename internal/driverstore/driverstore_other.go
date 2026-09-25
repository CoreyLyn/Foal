//go:build !windows

package driverstore

import "context"

// InspectDisplayPackages is unsupported off Windows.
func InspectDisplayPackages(context.Context) (Inventory, error) {
	return Inventory{}, ErrUnsupported
}

// RemovePackage is unsupported off Windows.
func RemovePackage(string) RemoveOutcome {
	return RemoveOutcomeFailed
}
