//go:build windows

package cli

import (
	"os/exec"
	"strings"
)

// copyTextToClipboard is a seam so tests can stub clipboard access.
var copyTextToClipboard = func(text string) error {
	// clip.exe ships with Windows and reads the clipboard payload from stdin.
	cmd := exec.Command("clip")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
