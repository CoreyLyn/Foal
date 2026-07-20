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
		OrphanedResidue: []OrphanedResidueCandidate{{
			Path:       `C:\Users\corey\AppData\Roaming\Gone App`,
			SourceRoot: `C:\Users\corey\AppData\Roaming`,
			Confidence: "low",
			Reason:     "directory is under an application data root but does not match a discovered installed application or publisher",
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
		"Orphaned residue",
		"Review only: low-confidence residue clues; not cleanup candidates.",
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
		"Orphaned residue",
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

func TestRenderPreviewReportShowsPlannedExecutionClassPerApplication(t *testing.T) {
	result := WithReviewSections(Result{
		Status: "preview",
		Applications: []Application{{
			Name:                        "Quiet App",
			QuietUninstallCommand:       `MsiExec.exe /X{GUID} /qn`,
			InteractiveUninstallCommand: `MsiExec.exe /X{GUID}`,
			InstallLocation:             `C:\Program Files\Quiet App`,
			PlannedClass:                PlannedClassOfficialUninstaller,
			PlannedReason:               "registry-advertised uninstall command is available",
			Evidence:                    []string{"windows_registry_uninstall_keys:HKLM64"},
			Confidence:                  "medium",
			Ownership:                   "unknown",
		}, {
			Name:            "Portable App",
			InstallLocation: `C:\Apps\PortableApp`,
			PlannedClass:    PlannedClassPortableDirectoryRemoval,
			PlannedReason:   "no uninstall command; install location is present",
			Evidence:        []string{"windows_registry_uninstall_keys:HKCU"},
			Confidence:      "medium",
			Ownership:       "unknown",
		}, {
			Name:          "Bare App",
			PlannedClass:  PlannedClassNotExecutable,
			PlannedReason: "no uninstall command and no install location",
			Evidence:      []string{"windows_registry_uninstall_keys:HKLM64"},
			Confidence:    "medium",
			Ownership:     "unknown",
		}, {
			Name:                  "Foal",
			QuietUninstallCommand: `MsiExec.exe /X{FOAL} /qn`,
			PlannedClass:          PlannedClassHardExclusion,
			PlannedReason:         "Foal never offers this application for Uninstall execution",
			Evidence:              []string{"windows_registry_uninstall_keys:HKLM64"},
			Confidence:            "medium",
			Ownership:             "unknown",
		}},
		Execution: previewOnlyExecution(),
	})

	output := RenderPreviewReport(result)

	for _, want := range []string{
		"Official uninstaller invocation",
		"Portable directory removal",
		"Not executable",
		"Uninstall hard exclusion",
		`C:\Program Files\Quiet App`,
		`MsiExec.exe /X{GUID} /qn`,
		`C:\Apps\PortableApp`,
		"Foal never offers this application for Uninstall execution",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
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
		EvidenceSources:     []EvidenceSource{{Source: orphanedResidueSource, Status: "reported"}},
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
	for _, section := range []string{"Possible leftovers", "Shared state concerns", "Orphaned residue", "Unknown state"} {
		sectionText := sectionText(t, output, section)
		if !strings.Contains(sectionText, "  none found") {
			t.Fatalf("%s missing none found empty state:\n%s", section, sectionText)
		}
	}
}

func TestRenderPreviewReportDistinguishesSkippedOrphanedResidueDiscovery(t *testing.T) {
	result := WithReviewSections(Result{
		Status:              "preview",
		Applications:        []Application{},
		EvidenceSources:     []EvidenceSource{{Source: orphanedResidueSource, Status: "skipped", Reason: "not running on Windows"}},
		PossibleLeftovers:   []LeftoverCandidate{},
		SharedStateConcerns: []SharedStateConcern{},
		OrphanedResidue:     []OrphanedResidueCandidate{},
		UnknownState:        []UnknownStateCandidate{},
		Skipped: []SkippedReason{{
			Source:      orphanedResidueSource,
			Reason:      "roots_not_configured",
			Recoverable: true,
		}},
		Execution: previewOnlyExecution(),
	})

	output := RenderPreviewReport(result)
	sectionText := sectionText(t, output, "Orphaned residue")
	if !strings.Contains(sectionText, "Not inspected: orphaned residue discovery was skipped") {
		t.Fatalf("orphaned residue section missing not-inspected state:\n%s", sectionText)
	}
	if strings.Contains(sectionText, "  none found") {
		t.Fatalf("orphaned residue section reported none found despite skipped discovery:\n%s", sectionText)
	}
}

func TestRenderPreviewReportTreatsEntrySkipsAsInspectedEmptyOrphanedResidue(t *testing.T) {
	result := WithReviewSections(Result{
		Status:              "preview",
		Applications:        []Application{},
		EvidenceSources:     []EvidenceSource{{Source: orphanedResidueSource, Status: "reported"}},
		PossibleLeftovers:   []LeftoverCandidate{},
		SharedStateConcerns: []SharedStateConcern{},
		OrphanedResidue:     []OrphanedResidueCandidate{},
		UnknownState:        []UnknownStateCandidate{},
		Skipped: []SkippedReason{{
			Source:      orphanedResidueSource,
			Reason:      "hidden_or_system",
			Recoverable: true,
		}},
		Execution: previewOnlyExecution(),
	})

	output := RenderPreviewReport(result)
	sectionText := sectionText(t, output, "Orphaned residue")
	if strings.Contains(sectionText, "Not inspected:") {
		t.Fatalf("orphaned residue section treated entry skip as provider skip:\n%s", sectionText)
	}
	if !strings.Contains(sectionText, "  none found") {
		t.Fatalf("orphaned residue section missing inspected-empty state:\n%s", sectionText)
	}
}

func TestRenderPreviewReportCapsEverySectionAtTenEntries(t *testing.T) {
	result := WithReviewSections(Result{
		Status:              "preview",
		Applications:        manyApplications(11),
		EvidenceSources:     manyEvidenceSources(11),
		PossibleLeftovers:   manyLeftovers(11),
		SharedStateConcerns: manySharedStateConcerns(11),
		OrphanedResidue:     manyOrphanedResidue(11),
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
		"Orphaned residue",
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

func TestReviewEvidencePopulatesPlannedClassAndUninstallEvidence(t *testing.T) {
	result := ReviewEvidence(Evidence{
		Applications: []ApplicationEvidence{
			{
				Name:                        "Installer App",
				QuietUninstallCommand:       `MsiExec.exe /X{A} /qn`,
				InteractiveUninstallCommand: `MsiExec.exe /X{A}`,
				InstallLocation:             `C:\Program Files\InstallerApp`,
				Sources:                     []string{"windows_registry_uninstall_keys:HKLM64"},
			},
			{
				Name:            "Portable App",
				InstallLocation: `C:\Apps\PortableApp`,
				Sources:         []string{"windows_registry_uninstall_keys:HKCU"},
			},
			{
				Name:    "Bare App",
				Sources: []string{"windows_registry_uninstall_keys:HKLM64"},
			},
			{
				Name:                  "Foal",
				QuietUninstallCommand: `MsiExec.exe /X{FOAL} /qn`,
				Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
			},
		},
	})

	if len(result.Applications) != 4 {
		t.Fatalf("applications = %d, want 4", len(result.Applications))
	}
	want := []struct {
		name, class, quiet, interactive, install string
	}{
		{"Installer App", PlannedClassOfficialUninstaller, `MsiExec.exe /X{A} /qn`, `MsiExec.exe /X{A}`, `C:\Program Files\InstallerApp`},
		{"Portable App", PlannedClassPortableDirectoryRemoval, "", "", `C:\Apps\PortableApp`},
		{"Bare App", PlannedClassNotExecutable, "", "", ""},
		// Foal is hard-excluded: it stays listed so the user sees Foal
		// recognized itself, but its executable command evidence is
		// suppressed so it is not presented as an executable target.
		{"Foal", PlannedClassHardExclusion, "", "", ""},
	}
	for i, w := range want {
		app := result.Applications[i]
		if app.Name != w.name {
			t.Fatalf("app[%d] name = %q, want %q", i, app.Name, w.name)
		}
		if app.PlannedClass != w.class {
			t.Fatalf("app[%d] %q planned_class = %q, want %q", i, w.name, app.PlannedClass, w.class)
		}
		if app.QuietUninstallCommand != w.quiet {
			t.Fatalf("app[%d] %q quiet = %q, want %q", i, w.name, app.QuietUninstallCommand, w.quiet)
		}
		if app.InteractiveUninstallCommand != w.interactive {
			t.Fatalf("app[%d] %q interactive = %q, want %q", i, w.name, app.InteractiveUninstallCommand, w.interactive)
		}
		if app.InstallLocation != w.install {
			t.Fatalf("app[%d] %q install = %q, want %q", i, w.name, app.InstallLocation, w.install)
		}
		if app.PlannedReason == "" {
			t.Fatalf("app[%d] %q planned reason = empty, want non-empty", i, w.name)
		}
	}
	// Hard exclusion must not be presented as an executable target even when a
	// quiet uninstall command is present.
	if result.Applications[3].PlannedClass != PlannedClassHardExclusion {
		t.Fatalf("Foal planned class = %q, want hard_exclusion", result.Applications[3].PlannedClass)
	}
	// Preview-only guarantee: review surfaces the plan but authorizes nothing.
	if result.Execution.Allowed {
		t.Fatal("execution allowed = true, want false")
	}
	if len(result.Execution.Actions) != 0 {
		t.Fatalf("execution actions = %#v, want empty", result.Execution.Actions)
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
		OrphanedResidue: []OrphanedResidueEvidence{{
			Path:       `C:\Users\corey\AppData\Local\Gone App`,
			SourceRoot: `C:\Users\corey\AppData\Local`,
		}},
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
	if len(result.OrphanedResidue) != 1 {
		t.Fatalf("orphaned residue = %#v, want one", result.OrphanedResidue)
	}
	if result.OrphanedResidue[0].Confidence != "low" || result.OrphanedResidue[0].SourceRoot == "" || result.OrphanedResidue[0].Reason == "" {
		t.Fatalf("orphaned residue = %#v, want low-confidence evidence", result.OrphanedResidue[0])
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
	stubOrphanedResidue(t, OrphanedResidueDiscoveryResult{
		Source: EvidenceSource{Source: orphanedResidueSource, Status: "reported"},
	})

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
	stubOrphanedResidue(t, OrphanedResidueDiscoveryResult{
		Candidates: []OrphanedResidueEvidence{{
			Path:       `C:\Users\corey\AppData\Local\Abandoned`,
			SourceRoot: `C:\Users\corey\AppData\Local`,
		}},
		Source: EvidenceSource{Source: orphanedResidueSource, Status: "reported"},
	})

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
	if len(result.OrphanedResidue) != 1 || result.OrphanedResidue[0].Confidence != "low" {
		t.Fatalf("orphaned residue = %#v, want one low-confidence candidate", result.OrphanedResidue)
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
	stubOrphanedResidue(t, OrphanedResidueDiscoveryResult{
		Source: EvidenceSource{Source: orphanedResidueSource, Status: "reported"},
	})

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
	stubOrphanedResidue(t, OrphanedResidueDiscoveryResult{
		Source:  EvidenceSource{Source: orphanedResidueSource, Status: "skipped", Reason: "not running on Windows"},
		Skipped: []SkippedReason{{Source: orphanedResidueSource, Reason: "unsupported_platform", Recoverable: true}},
	})

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
	if !hasSkippedSource(result.Skipped, orphanedResidueSource) {
		t.Fatalf("skipped = %#v, want orphaned_residue unsupported_platform skip", result.Skipped)
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

func stubOrphanedResidue(t *testing.T, result OrphanedResidueDiscoveryResult) {
	t.Helper()

	original := discoverOrphanedResidueEvidence
	discoverOrphanedResidueEvidence = func([]ApplicationEvidence) OrphanedResidueDiscoveryResult {
		return result
	}
	t.Cleanup(func() { discoverOrphanedResidueEvidence = original })
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

func TestReviewWiresOrphanedResidueProviderWithRegistryDiscovery(t *testing.T) {
	original := discoverUninstallEvidence
	discoverUninstallEvidence = func() DiscoveryResult {
		return DiscoveryResult{
			Evidence: Evidence{
				Applications: []ApplicationEvidence{{
					Name:      "Registry App",
					Publisher: "Registry Publisher",
					Sources:   []string{"windows_registry_uninstall_keys:HKLM64"},
				}},
			},
			Sources: []EvidenceSource{{Source: "windows_registry_uninstall_keys:HKLM64", Status: "reported"}},
		}
	}
	t.Cleanup(func() { discoverUninstallEvidence = original })
	originalLeftover := discoverLeftoverEvidence
	discoverLeftoverEvidence = func([]ApplicationEvidence) LeftoverDiscoveryResult {
		return LeftoverDiscoveryResult{Source: EvidenceSource{Source: "known_leftover_locations", Status: "reported"}}
	}
	t.Cleanup(func() { discoverLeftoverEvidence = originalLeftover })

	originalOrphaned := discoverOrphanedResidueEvidence
	discoverOrphanedResidueEvidence = func(apps []ApplicationEvidence) OrphanedResidueDiscoveryResult {
		if len(apps) != 1 || apps[0].Name != "Registry App" || apps[0].Publisher != "Registry Publisher" {
			t.Fatalf("orphaned provider received apps = %#v, want registry-discovered app", apps)
		}
		return OrphanedResidueDiscoveryResult{
			Candidates: []OrphanedResidueEvidence{{
				Path:       `C:\Users\corey\AppData\Roaming\Gone App`,
				SourceRoot: `C:\Users\corey\AppData\Roaming`,
			}},
			Source: EvidenceSource{Source: orphanedResidueSource, Status: "reported"},
		}
	}
	t.Cleanup(func() { discoverOrphanedResidueEvidence = originalOrphaned })

	result := Review()

	if len(result.OrphanedResidue) != 1 {
		t.Fatalf("orphaned residue = %#v, want one candidate", result.OrphanedResidue)
	}
	got := result.OrphanedResidue[0]
	if got.Confidence != "low" || got.SourceRoot == "" || got.Reason == "" {
		t.Fatalf("orphaned residue = %#v, want low-confidence automation evidence", got)
	}
	if result.Execution.Allowed {
		t.Fatal("execution allowed = true, want false")
	}
	if len(result.Execution.Actions) != 0 {
		t.Fatalf("execution actions = %#v, want empty", result.Execution.Actions)
	}
}

func manyOrphanedResidue(count int) []OrphanedResidueCandidate {
	candidates := make([]OrphanedResidueCandidate, 0, count)
	for i := 1; i <= count; i++ {
		candidates = append(candidates, OrphanedResidueCandidate{
			Path:       fmt.Sprintf(`C:\Users\corey\AppData\Roaming\Gone%02d`, i),
			SourceRoot: `C:\Users\corey\AppData\Roaming`,
			Confidence: "low",
			Reason:     "directory is under an application data root but does not match a discovered installed application or publisher",
		})
	}
	return candidates
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
