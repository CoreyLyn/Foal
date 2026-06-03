//go:build !windows

package delete

import "io/fs"

type WindowsRecycleBinAdapter struct{}

func (WindowsRecycleBinAdapter) MoveToRecycleBin(string) error {
	return fs.ErrInvalid
}
