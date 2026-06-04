//go:build !windows

package uninstall

func discoverPlatformUninstallEvidence() DiscoveryResult {
	return DiscoveryResult{
		Sources: []EvidenceSource{{
			Source: "windows_registry_uninstall_keys",
			Status: "skipped",
			Reason: "not running on Windows",
		}},
		Skipped: []SkippedReason{{
			Source:      "windows_registry_uninstall_keys",
			Reason:      "unsupported_platform",
			Recoverable: true,
		}},
	}
}
