package uninstall

import (
	"strings"
	"testing"
)

func TestRenderPreviewReportShowsCoreSectionsInReviewOrder(t *testing.T) {
	result := WithReviewSections(Result{
		Status: "preview",
		Applications: []Application{{
			Name:       "Example App",
			Version:    "1.2.3",
			Publisher:  "Example Co",
			Evidence:   []string{"windows_registry_uninstall_keys:HKLM64"},
			Confidence: "medium",
			Ownership:  "unknown",
		}},
		EvidenceSources: []EvidenceSource{{
			Source: "windows_registry_uninstall_keys:HKLM64",
			Status: "reported",
		}},
		PossibleLeftovers: []LeftoverCandidate{{
			Path:       `C:\Users\corey\AppData\Local\Example App`,
			App:        "Example App",
			Ownership:  "app_owned",
			Confidence: "high",
			Reason:     "leftover signals tie this path to one application",
		}},
		SharedStateConcerns: []SharedStateConcern{{
			Path:   `C:\ProgramData\Example Co`,
			Reason: "candidate appears to contain shared application or publisher state",
		}},
		UnknownState: []UnknownStateCandidate{{
			Path:   `C:\Users\corey\AppData\Roaming\mystery`,
			Reason: "evidence is too weak for an ownership decision",
		}},
		Skipped: []SkippedReason{{
			Source:      "known_leftover_locations",
			Reason:      "discovery_provider_not_implemented",
			Recoverable: true,
		}},
		Execution: ExecutionPolicy{
			Allowed: false,
			Actions: []string{},
			Reason:  "uninstall is preview-only; Foal does not execute uninstallers, stop processes, or delete leftovers",
		},
	})

	output := RenderPreviewReport(result)

	for _, want := range []string{
		"Foal uninstall",
		"Preview only",
		"uninstall is preview-only; Foal does not execute uninstallers, stop processes, or delete leftovers",
		"Applications",
		"Evidence sources",
		"Possible leftovers",
		"Shared state concerns",
		"Unknown state",
		"Skipped discovery sources",
		"Summary:",
		"Example App",
		`C:\Users\corey\AppData\Local\Example App`,
		"windows_registry_uninstall_keys:HKLM64",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}

	assertOrdered(t, output, []string{
		"Applications",
		"Evidence sources",
		"Possible leftovers",
		"Shared state concerns",
		"Unknown state",
		"Skipped discovery sources",
		"Summary:",
	})
	assertASCII(t, output)
	for _, forbidden := range []string{
		"foal uninstall --execute",
		"Run uninstaller",
		"Stop process",
		"Delete leftover",
		"Actions:",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output contains unsupported execution wording %q:\n%s", forbidden, output)
		}
	}
}

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

func TestReviewKeepsKnownLeftoverProviderSkippedAfterRegistryDiscovery(t *testing.T) {
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
	if result.EvidenceSources[1].Source != "known_leftover_locations" || result.EvidenceSources[1].Status != "skipped" {
		t.Fatalf("leftover evidence source = %#v, want skipped known_leftover_locations", result.EvidenceSources[1])
	}
	if result.EvidenceSources[1].Reason != "discovery provider not implemented" {
		t.Fatalf("leftover evidence source reason = %q, want discovery provider not implemented", result.EvidenceSources[1].Reason)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("skipped = %#v, want known leftover discovery skip only", result.Skipped)
	}
	if result.Skipped[0].Source != "known_leftover_locations" || result.Skipped[0].Reason != "discovery_provider_not_implemented" || !result.Skipped[0].Recoverable {
		t.Fatalf("skipped[0] = %#v, want recoverable known leftover discovery skip", result.Skipped[0])
	}
	if len(result.PossibleLeftovers) != 0 {
		t.Fatalf("possible leftovers = %#v, want empty after registry metadata discovery", result.PossibleLeftovers)
	}
	if len(result.SharedStateConcerns) != 0 {
		t.Fatalf("shared state concerns = %#v, want empty after registry metadata discovery", result.SharedStateConcerns)
	}
	if len(result.UnknownState) != 0 {
		t.Fatalf("unknown state = %#v, want empty after registry metadata discovery", result.UnknownState)
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

func TestReviewReportsUnsupportedPlatformAsPreviewOnlyRecoverableSkip(t *testing.T) {
	originalDiscover := discoverUninstallEvidence
	discoverUninstallEvidence = func() DiscoveryResult {
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
	t.Cleanup(func() { discoverUninstallEvidence = originalDiscover })

	originalGOOS := runtimeGOOS
	runtimeGOOS = "linux"
	t.Cleanup(func() { runtimeGOOS = originalGOOS })

	result := Review()

	if len(result.Applications) != 0 {
		t.Fatalf("applications = %#v, want none on unsupported platform", result.Applications)
	}
	if !hasSkippedSource(result.Skipped, "windows_registry_uninstall_keys") {
		t.Fatalf("skipped = %#v, want registry unsupported_platform skip", result.Skipped)
	}
	if !hasSkippedSource(result.Skipped, "windows_only_evidence") {
		t.Fatalf("skipped = %#v, want windows_only_evidence unsupported_platform skip", result.Skipped)
	}
	if result.Execution.Allowed {
		t.Fatal("execution allowed = true, want false")
	}
	if len(result.Execution.Actions) != 0 {
		t.Fatalf("execution actions = %#v, want empty", result.Execution.Actions)
	}
}

func assertOrdered(t *testing.T, text string, values []string) {
	t.Helper()

	previous := -1
	for _, value := range values {
		index := strings.Index(text, value)
		if index == -1 {
			t.Fatalf("missing ordered value %q in:\n%s", value, text)
		}
		if index < previous {
			t.Fatalf("%q appeared before prior section in:\n%s", value, text)
		}
		previous = index
	}
}

func assertASCII(t *testing.T, text string) {
	t.Helper()

	for _, r := range text {
		if r > 127 {
			t.Fatalf("output contains non-ASCII rune %q:\n%s", r, text)
		}
	}
}
