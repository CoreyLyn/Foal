//go:build !windows

package status

import "errors"

func captureDisk(path string) (DiskSnapshot, error) {
	return DiskSnapshot{Path: path}, errors.New("disk capacity snapshot is only implemented on Windows")
}
