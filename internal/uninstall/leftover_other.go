//go:build !windows

package uninstall

// discoverPlatformLeftoverEvidence reports the footprint provider as a
// recoverable skip off Windows, mirroring the registry provider so the preview
// stays stable and honest about what it did not inspect on this platform.
func discoverPlatformLeftoverEvidence(apps []ApplicationEvidence) LeftoverDiscoveryResult {
	return LeftoverDiscoveryResult{
		Source: EvidenceSource{
			Source: "known_leftover_locations",
			Status: "skipped",
			Reason: "not running on Windows",
		},
		Skipped: []SkippedReason{{
			Source:      "known_leftover_locations",
			Reason:      "unsupported_platform",
			Recoverable: true,
		}},
	}
}
