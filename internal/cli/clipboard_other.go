//go:build !windows

package cli

import "errors"

// copyTextToClipboard is a seam so tests can stub clipboard access.
var copyTextToClipboard = func(text string) error {
	return errors.New("clipboard copy is only supported on Windows")
}
