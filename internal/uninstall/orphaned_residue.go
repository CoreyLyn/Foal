package uninstall

import (
	"path/filepath"
	"strings"
)

const orphanedResidueSource = "orphaned_residue"

var discoverOrphanedResidueEvidence = discoverPlatformOrphanedResidueEvidence

type OrphanedResidueDiscoveryResult struct {
	Candidates []OrphanedResidueEvidence
	Source     EvidenceSource
	Skipped    []SkippedReason
}

type orphanedResidueEntry struct {
	Name   string
	Skip   bool
	Reason string
}

type orphanedResidueLister func(root string) ([]orphanedResidueEntry, error)

func probeOrphanedResidue(apps []ApplicationEvidence, roots []string, list orphanedResidueLister) OrphanedResidueDiscoveryResult {
	if len(roots) == 0 {
		return OrphanedResidueDiscoveryResult{
			Source:  EvidenceSource{Source: orphanedResidueSource, Status: "skipped", Reason: "no application data roots configured"},
			Skipped: []SkippedReason{{Source: orphanedResidueSource, Reason: "roots_not_configured", Recoverable: true}},
		}
	}

	excluded := installedNameIndex(apps)
	for name := range sharedResidueNames {
		excluded[name] = true
	}

	var out []OrphanedResidueEvidence
	var skipped []SkippedReason
	inspected := false
	seen := map[string]bool{}

	for _, root := range roots {
		entries, err := list(root)
		if err != nil {
			skipped = append(skipped, SkippedReason{Source: orphanedResidueSource, Reason: "root_unreadable", Recoverable: true})
			continue
		}
		inspected = true
		for _, entry := range entries {
			if entry.Skip {
				reason := entry.Reason
				if reason == "" {
					reason = "directory_not_safe_to_classify"
				}
				skipped = append(skipped, SkippedReason{Source: orphanedResidueSource, Reason: reason, Recoverable: true})
				continue
			}
			key := normalizeName(entry.Name)
			if key == "" || excluded[key] {
				continue
			}
			path := filepath.Join(root, entry.Name)
			seenKey := strings.ToLower(path)
			if seen[seenKey] {
				continue
			}
			seen[seenKey] = true
			out = append(out, OrphanedResidueEvidence{Path: path, SourceRoot: root})
		}
	}

	if !inspected {
		return OrphanedResidueDiscoveryResult{
			Source:  EvidenceSource{Source: orphanedResidueSource, Status: "skipped", Reason: "application data roots were not inspected"},
			Skipped: skipped,
		}
	}

	return OrphanedResidueDiscoveryResult{
		Candidates: out,
		Source:     EvidenceSource{Source: orphanedResidueSource, Status: "reported"},
		Skipped:    skipped,
	}
}

func installedNameIndex(apps []ApplicationEvidence) map[string]bool {
	out := map[string]bool{}
	for _, app := range apps {
		for _, name := range []string{app.Name, app.Publisher} {
			key := normalizeName(name)
			if key != "" {
				out[key] = true
			}
		}
	}
	return out
}

var sharedResidueNames = map[string]bool{
	"microsoft": true,
	"google":    true,
	"jetbrains": true,
	"intel":     true,
	"lenovo":    true,
	"mozilla":   true,
	"adobe":     true,
	"nvidia":    true,
	"packages":  true,
	"temp":      true,
}
