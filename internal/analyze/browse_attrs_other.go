//go:build !windows

package analyze

import "os"

func platformPresentationAttributes(path string, info os.FileInfo) presentationAttributes {
	_ = path
	attrs := presentationAttributes{}
	if info != nil && info.Mode()&os.ModeSymlink != 0 {
		attrs.Reparse = true
	}
	return attrs
}
