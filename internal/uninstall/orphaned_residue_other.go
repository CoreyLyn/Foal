//go:build !windows

package uninstall

func discoverPlatformOrphanedResidueEvidence([]ApplicationEvidence) OrphanedResidueDiscoveryResult {
	return OrphanedResidueDiscoveryResult{
		Source:  EvidenceSource{Source: orphanedResidueSource, Status: "skipped", Reason: "not running on Windows"},
		Skipped: []SkippedReason{{Source: orphanedResidueSource, Reason: "unsupported_platform", Recoverable: true}},
	}
}
