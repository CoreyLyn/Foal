//go:build windows

package uninstall

import (
	"os"
	"syscall"
)

func discoverPlatformOrphanedResidueEvidence(apps []ApplicationEvidence) OrphanedResidueDiscoveryResult {
	return probeOrphanedResidue(apps, orphanedResidueRoots(), listOrphanedResidueRoot)
}

func orphanedResidueRoots() []string {
	var roots []string
	for _, env := range []string{"APPDATA", "LOCALAPPDATA"} {
		if path := os.Getenv(env); path != "" {
			roots = append(roots, path)
		}
	}
	return roots
}

func listOrphanedResidueRoot(root string) ([]orphanedResidueEntry, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []orphanedResidueEntry
	for _, entry := range entries {
		item := orphanedResidueEntry{Name: entry.Name()}
		if entry.Type()&os.ModeSymlink != 0 {
			item.Skip = true
			item.Reason = "reparse_point"
			out = append(out, item)
			continue
		}
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			item.Skip = true
			item.Reason = "directory_unreadable"
			out = append(out, item)
			continue
		}
		mode := info.Mode()
		if mode&os.ModeSymlink != 0 {
			item.Skip = true
			item.Reason = "reparse_point"
			out = append(out, item)
			continue
		}
		if attrs, ok := windowsFileAttributes(info); ok {
			if attrs&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
				item.Skip = true
				item.Reason = "reparse_point"
				out = append(out, item)
				continue
			}
			if attrs&(syscall.FILE_ATTRIBUTE_HIDDEN|syscall.FILE_ATTRIBUTE_SYSTEM) != 0 {
				item.Skip = true
				item.Reason = "hidden_or_system"
				out = append(out, item)
				continue
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func windowsFileAttributes(info os.FileInfo) (uint32, bool) {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return 0, false
	}
	return data.FileAttributes, true
}
