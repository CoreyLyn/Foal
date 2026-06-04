package uninstall

import "testing"

func TestReviewEvidenceClassifiesApplicationsAndLeftovers(t *testing.T) {
	result := ReviewEvidence(Evidence{
		Applications: []ApplicationEvidence{
			{
				Name:      "Example App",
				Version:   "1.2.3",
				Publisher: "Example Co",
				Sources:   []string{"windows_registry_uninstall_keys", "install_location"},
			},
		},
		Leftovers: []LeftoverEvidence{
			{Path: `C:\Users\corey\AppData\Local\Example App\cache`, App: "Example App", Signals: []string{"app_name_match", "under_user_profile"}},
			{Path: `C:\ProgramData\Example Co\Shared`, App: "Example App", Signals: []string{"shared_program_data"}},
			{Path: `C:\Users\corey\AppData\Roaming\mystery`, Signals: []string{"weak_name_match"}},
		},
	})

	if len(result.Applications) != 1 {
		t.Fatalf("applications = %#v, want one", result.Applications)
	}
	app := result.Applications[0]
	if app.Confidence != "high" {
		t.Fatalf("app confidence = %q, want high", app.Confidence)
	}
	if app.Ownership != "app_owned" {
		t.Fatalf("app ownership = %q, want app_owned", app.Ownership)
	}
	if len(result.PossibleLeftovers) != 1 {
		t.Fatalf("possible leftovers = %#v, want one app-owned candidate", result.PossibleLeftovers)
	}
	if result.PossibleLeftovers[0].Ownership != "app_owned" || result.PossibleLeftovers[0].Confidence != "high" {
		t.Fatalf("leftover classification = %#v, want app_owned/high", result.PossibleLeftovers[0])
	}
	if len(result.SharedStateConcerns) != 1 {
		t.Fatalf("shared state concerns = %#v, want one", result.SharedStateConcerns)
	}
	if len(result.UnknownState) != 1 {
		t.Fatalf("unknown state = %#v, want one", result.UnknownState)
	}
	if result.Execution.Allowed {
		t.Fatal("execution allowed = true, want false")
	}
	if len(result.Execution.Actions) != 0 {
		t.Fatalf("execution actions = %#v, want empty", result.Execution.Actions)
	}
}

func TestReviewReportsKnownLeftoverProviderAsSkipped(t *testing.T) {
	result := Review()

	found := false
	for _, skipped := range result.Skipped {
		if skipped.Source == "known_leftover_locations" {
			found = true
			if skipped.Reason != "discovery_provider_not_implemented" {
				t.Fatalf("known leftover skipped reason = %q, want discovery_provider_not_implemented", skipped.Reason)
			}
			if !skipped.Recoverable {
				t.Fatalf("known leftover skipped recoverable = false for %#v, want true", skipped)
			}
		}
	}
	if !found {
		t.Fatalf("skipped = %#v, want known leftover discovery skip", result.Skipped)
	}
	if result.Execution.Allowed {
		t.Fatal("execution allowed = true, want false")
	}
}

func TestReviewReportsRegistryDiscoveredApplicationsWithoutExecution(t *testing.T) {
	original := discoverUninstallEvidence
	discoverUninstallEvidence = func() DiscoveryResult {
		return DiscoveryResult{
			Evidence: Evidence{
				Applications: []ApplicationEvidence{{
					Name:      "Registry App",
					Version:   "2.4.6",
					Publisher: "Registry Publisher",
					Sources:   []string{"windows_registry_uninstall_keys:HKLM64"},
				}},
			},
			Sources: []EvidenceSource{{
				Source: "windows_registry_uninstall_keys:HKLM64",
				Status: "reported",
			}},
		}
	}
	t.Cleanup(func() { discoverUninstallEvidence = original })

	result := Review()

	if len(result.Applications) != 1 {
		t.Fatalf("applications = %#v, want one registry-discovered app", result.Applications)
	}
	app := result.Applications[0]
	if app.Name != "Registry App" || app.Version != "2.4.6" || app.Publisher != "Registry Publisher" {
		t.Fatalf("app = %#v, want name/version/publisher from registry metadata", app)
	}
	if len(app.Evidence) != 1 || app.Evidence[0] != "windows_registry_uninstall_keys:HKLM64" {
		t.Fatalf("app evidence = %#v, want registry source metadata", app.Evidence)
	}
	if len(result.EvidenceSources) != 2 {
		t.Fatalf("evidence sources = %#v, want registry reported plus leftover skipped", result.EvidenceSources)
	}
	if result.EvidenceSources[0].Source != "windows_registry_uninstall_keys:HKLM64" || result.EvidenceSources[0].Status != "reported" {
		t.Fatalf("registry evidence source = %#v, want reported HKLM64 source", result.EvidenceSources[0])
	}
	if result.Execution.Allowed {
		t.Fatal("execution allowed = true, want false")
	}
	if len(result.Execution.Actions) != 0 {
		t.Fatalf("execution actions = %#v, want empty", result.Execution.Actions)
	}
}

func TestReviewReportsRegistryDiscoveryFailuresAsRecoverableSkips(t *testing.T) {
	original := discoverUninstallEvidence
	discoverUninstallEvidence = func() DiscoveryResult {
		return DiscoveryResult{
			Sources: []EvidenceSource{{
				Source: "windows_registry_uninstall_keys:HKCU",
				Status: "skipped",
				Reason: "registry access denied",
			}},
			Skipped: []SkippedReason{{
				Source:      "windows_registry_uninstall_keys:HKCU",
				Reason:      "registry_discovery_failed",
				Recoverable: true,
			}},
		}
	}
	t.Cleanup(func() { discoverUninstallEvidence = original })

	result := Review()

	if len(result.Applications) != 0 {
		t.Fatalf("applications = %#v, want none after provider failure", result.Applications)
	}
	if len(result.Skipped) == 0 {
		t.Fatalf("skipped = %#v, want recoverable provider failure", result.Skipped)
	}
	if result.Skipped[0].Source != "windows_registry_uninstall_keys:HKCU" || result.Skipped[0].Reason != "registry_discovery_failed" || !result.Skipped[0].Recoverable {
		t.Fatalf("skipped[0] = %#v, want recoverable registry_discovery_failed", result.Skipped[0])
	}
	if result.Execution.Allowed {
		t.Fatal("execution allowed = true, want false")
	}
	if len(result.Execution.Actions) != 0 {
		t.Fatalf("execution actions = %#v, want empty", result.Execution.Actions)
	}
}
