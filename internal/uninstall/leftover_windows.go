//go:build windows

package uninstall

import (
	"os"
)

// discoverPlatformLeftoverEvidence probes the Windows per-user and machine-shared
// data roots for the footprint of already-discovered applications. It is
// read-only: it lists directories and never reads registry values, writes to
// disk, or executes uninstall strings.
func discoverPlatformLeftoverEvidence(apps []ApplicationEvidence) LeftoverDiscoveryResult {
	leftovers, _ := probeFootprint(apps, footprintRoots(), listSubdirectories)
	return LeftoverDiscoveryResult{
		Leftovers: leftovers,
		Source:    EvidenceSource{Source: "known_leftover_locations", Status: "reported"},
	}
}

// footprintRoots resolves the high-signal data roots from the environment. A
// root whose variable is unset is simply skipped.
func footprintRoots() []rootSpec {
	var roots []rootSpec
	for _, root := range []struct {
		env          string
		underProfile bool
	}{
		{env: "APPDATA", underProfile: true},      // Roaming
		{env: "LOCALAPPDATA", underProfile: true}, // Local
		{env: "PROGRAMDATA", underProfile: false}, // machine-shared
	} {
		if path := os.Getenv(root.env); path != "" {
			roots = append(roots, rootSpec{path: path, underProfile: root.underProfile})
		}
	}
	return roots
}

// listSubdirectories returns the immediate subdirectory names of dir. A missing
// directory is reported as empty rather than an error so probing continues.
func listSubdirectories(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}
