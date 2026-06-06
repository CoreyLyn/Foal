package uninstall

import (
	"fmt"
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

func TestRenderPreviewReportUsesCapabilityAwareEmptyStatesAndConfidenceNote(t *testing.T) {
	result := WithReviewSections(Result{
		Status: "preview",
		Applications: []Application{{
			Name:       "Example App",
			Evidence:   []string{"windows_registry_uninstall_keys:HKLM64"},
			Confidence: "medium",
			Ownership:  "unknown",
		}},
		EvidenceSources: []EvidenceSource{{
			Source: "windows_registry_uninstall_keys:HKLM64",
			Status: "reported",
		}, {
			Source: "known_leftover_locations",
			Status: "skipped",
			Reason: "discovery provider not implemented",
		}},
		PossibleLeftovers:   []LeftoverCandidate{},
		SharedStateConcerns: []SharedStateConcern{},
		UnknownState:        []UnknownStateCandidate{},
		Skipped: []SkippedReason{{
			Source:      "known_leftover_locations",
			Reason:      "discovery_provider_not_implemented",
			Recoverable: true,
		}},
		Execution: previewOnlyExecution(),
	})

	output := RenderPreviewReport(result)

	for _, want := range []string{
		"Applications are reported at medium confidence; high-confidence ownership requires multi-source evidence, not implemented yet",
		"Not inspected: leftover discovery is not implemented yet (see Skipped discovery sources)",
		"Skipped discovery sources",
		"known_leftover_locations",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if got := strings.Count(output, "Not inspected: leftover discovery is not implemented yet (see Skipped discovery sources)"); got != 3 {
		t.Fatalf("not-inspected lines = %d, want 3:\n%s", got, output)
	}
	for _, forbidden := range []string{
		"    confidence:",
		"    ownership:",
		"  - known_leftover_locations\n    status: skipped",
		"None reported",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output contains forbidden %q:\n%s", forbidden, output)
		}
	}
}

func TestRenderPreviewReportSaysNoneFoundWhenLeftoversWereInspectedAndEmpty(t *testing.T) {
	result := WithReviewSections(Result{
		Status:              "preview",
		Applications:        []Application{},
		EvidenceSources:     []EvidenceSource{},
		PossibleLeftovers:   []LeftoverCandidate{},
		SharedStateConcerns: []SharedStateConcern{},
		UnknownState:        []UnknownStateCandidate{},
		Skipped:             []SkippedReason{},
		Execution:           previewOnlyExecution(),
	})

	output := RenderPreviewReport(result)

	if strings.Contains(output, "Not inspected:") {
		t.Fatalf("output contains not-inspected state for inspected empty leftovers:\n%s", output)
	}
	for _, section := range []string{"Possible leftovers", "Shared state concerns", "Unknown state"} {
		sectionText := sectionText(t, output, section)
		if !strings.Contains(sectionText, "  none found") {
			t.Fatalf("%s missing none found empty state:\n%s", section, sectionText)
		}
	}
}

func TestRenderPreviewReportCapsEverySectionAtTenEntries(t *testing.T) {
	result := WithReviewSections(Result{
		Status:              "preview",
		Applications:        manyApplications(11),
		EvidenceSources:     manyEvidenceSources(11),
		PossibleLeftovers:   manyLeftovers(11),
		SharedStateConcerns: manySharedStateConcerns(11),
		UnknownState:        manyUnknownState(11),
		Skipped:             manySkippedReasons(11),
		Execution:           previewOnlyExecution(),
	})

	output := RenderPreviewReport(result)

	for _, section := range []string{
		"Applications",
		"Evidence sources (reported)",
		"Possible leftovers",
		"Shared state concerns",
		"Unknown state",
		"Skipped discovery sources",
	} {
		sectionText := sectionText(t, output, section)
		if got := strings.Count(sectionText, "  - "); got != 10 {
			t.Fatalf("%s entries = %d, want 10:\n%s", section, got, sectionText)
		}
		if !strings.Contains(sectionText, "  1 omitted. See foal uninstall --json.") {
			t.Fatalf("%s missing JSON overflow line:\n%s", section, sectionText)
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

func TestReviewSurfacesLeftoverProviderSkip(t *testing.T) {
	original := discoverUninstallEvidence
	discoverUninstallEvidence = func() DiscoveryResult { return DiscoveryResult{} }
	t.Cleanup(func() { discoverUninstallEvidence = original })

	originalLeftover := discoverLeftoverEvidence
	discoverLeftoverEvidence = func([]ApplicationEvidence) LeftoverDiscoveryResult {
		return LeftoverDiscoveryResult{
			Source:  EvidenceSource{Source: "known_leftover_locations", Status: "skipped", Reason: "not running on Windows"},
			Skipped: []SkippedReason{{Source: "known_leftover_locations", Reason: "unsupported_platform", Recoverable: true}},
		}
	}
	t.Cleanup(func() { discoverLeftoverEvidence = originalLeftover })

	result := Review()

	found := false
	for _, skipped := range result.Skipped {
		if skipped.Source == "known_leftover_locations" {
			found = true
			if !skipped.Recoverable {
				t.Fatalf("known leftover skip recoverable = false for %#v, want true", skipped)
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

func TestReviewWiresReportedLeftoverProviderWithRegistryDiscovery(t *testing.T) {
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

	originalLeftover := discoverLeftoverEvidence
	discoverLeftoverEvidence = func(apps []ApplicationEvidence) LeftoverDiscoveryResult {
		if len(apps) != 1 || apps[0].Name != "Registry App" {
			t.Fatalf("leftover provider received apps = %#v, want the registry-discovered app", apps)
		}
		return LeftoverDiscoveryResult{
			Leftovers: []LeftoverEvidence{
				{Path: `C:\Users\corey\AppData\Roaming\Registry App`, App: "Registry App", Signals: []string{"app_name_match", "under_user_profile"}},
				{Path: `C:\ProgramData\Registry Publisher`, App: "Registry App", Signals: []string{"shared_program_data"}},
			},
			Source: EvidenceSource{Source: "known_leftover_locations", Status: "reported"},
		}
	}
	t.Cleanup(func() { discoverLeftoverEvidence = originalLeftover })

	result := Review()

	if len(result.Applications) != 1 || result.Applications[0].Name != "Registry App" {
		t.Fatalf("applications = %#v, want one registry-discovered app", result.Applications)
	}
	if len(result.PossibleLeftovers) != 1 {
		t.Fatalf("possible leftovers = %#v, want one app-owned footprint", result.PossibleLeftovers)
	}
	if result.PossibleLeftovers[0].Ownership != "app_owned" || result.PossibleLeftovers[0].Confidence != "high" {
		t.Fatalf("possible leftover = %#v, want app_owned/high", result.PossibleLeftovers[0])
	}
	if len(result.SharedStateConcerns) != 1 {
		t.Fatalf("shared state concerns = %#v, want one shared publisher path", result.SharedStateConcerns)
	}
	if len(result.UnknownState) != 0 {
		t.Fatalf("unknown state = %#v, want empty in footprint slice", result.UnknownState)
	}
	if hasSkippedSource(result.Skipped, "known_leftover_locations") {
		t.Fatalf("skipped = %#v, want known_leftover_locations reported, not skipped", result.Skipped)
	}
	reported := false
	for _, source := range result.EvidenceSources {
		if source.Source == "known_leftover_locations" {
			if source.Status != "reported" {
				t.Fatalf("known_leftover_locations source = %#v, want reported", source)
			}
			reported = true
		}
	}
	if !reported {
		t.Fatalf("evidence sources = %#v, want known_leftover_locations reported", result.EvidenceSources)
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

	originalLeftover := discoverLeftoverEvidence
	discoverLeftoverEvidence = func([]ApplicationEvidence) LeftoverDiscoveryResult {
		return LeftoverDiscoveryResult{
			Source:  EvidenceSource{Source: "known_leftover_locations", Status: "skipped", Reason: "not running on Windows"},
			Skipped: []SkippedReason{{Source: "known_leftover_locations", Reason: "unsupported_platform", Recoverable: true}},
		}
	}
	t.Cleanup(func() { discoverLeftoverEvidence = originalLeftover })

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
	if !hasSkippedSource(result.Skipped, "known_leftover_locations") {
		t.Fatalf("skipped = %#v, want known_leftover_locations unsupported_platform skip", result.Skipped)
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

func sectionText(t *testing.T, output, section string) string {
	t.Helper()

	start := strings.Index(output, section+"\n")
	if start == -1 {
		t.Fatalf("missing section %q in:\n%s", section, output)
	}
	rest := output[start:]
	end := strings.Index(rest, "\n\n")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

func manyApplications(count int) []Application {
	applications := make([]Application, 0, count)
	for i := 1; i <= count; i++ {
		applications = append(applications, Application{
			Name:       fmt.Sprintf("App %02d", i),
			Evidence:   []string{"windows_registry_uninstall_keys:HKLM64"},
			Confidence: "medium",
			Ownership:  "unknown",
		})
	}
	return applications
}

func manyEvidenceSources(count int) []EvidenceSource {
	sources := make([]EvidenceSource, 0, count)
	for i := 1; i <= count; i++ {
		sources = append(sources, EvidenceSource{
			Source: fmt.Sprintf("source_%02d", i),
			Status: "reported",
		})
	}
	return sources
}

func manyLeftovers(count int) []LeftoverCandidate {
	leftovers := make([]LeftoverCandidate, 0, count)
	for i := 1; i <= count; i++ {
		leftovers = append(leftovers, LeftoverCandidate{
			Path:       fmt.Sprintf(`C:\Users\corey\AppData\Local\App%02d`, i),
			App:        fmt.Sprintf("App %02d", i),
			Ownership:  "app_owned",
			Confidence: "high",
			Reason:     "leftover signals tie this path to one application",
		})
	}
	return leftovers
}

func manySharedStateConcerns(count int) []SharedStateConcern {
	concerns := make([]SharedStateConcern, 0, count)
	for i := 1; i <= count; i++ {
		concerns = append(concerns, SharedStateConcern{
			Path:   fmt.Sprintf(`C:\ProgramData\Shared%02d`, i),
			Reason: "candidate appears to contain shared application or publisher state",
		})
	}
	return concerns
}

func manyUnknownState(count int) []UnknownStateCandidate {
	unknown := make([]UnknownStateCandidate, 0, count)
	for i := 1; i <= count; i++ {
		unknown = append(unknown, UnknownStateCandidate{
			Path:   fmt.Sprintf(`C:\Users\corey\AppData\Roaming\Mystery%02d`, i),
			Reason: "evidence is too weak for an ownership decision",
		})
	}
	return unknown
}

func manySkippedReasons(count int) []SkippedReason {
	skipped := make([]SkippedReason, 0, count)
	for i := 1; i <= count; i++ {
		skipped = append(skipped, SkippedReason{
			Source:      fmt.Sprintf("provider_%02d", i),
			Reason:      "discovery_provider_not_implemented",
			Recoverable: true,
		})
	}
	return skipped
}
