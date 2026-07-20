package uninstall

import (
	"strings"
	"testing"
)

// TestSourcesRequireAdminDetectsHKLMSources verifies the conservative
// detection helper: HKLM64/HKLM32 sources indicate machine-wide installs
// that likely need admin; HKCU sources do not.
func TestSourcesRequireAdminDetectsHKLMSources(t *testing.T) {
	tests := []struct {
		name    string
		sources []string
		want    bool
	}{
		{"HKLM64 only", []string{"windows_registry_uninstall_keys:HKLM64"}, true},
		{"HKLM32 only", []string{"windows_registry_uninstall_keys:HKLM32"}, true},
		{"HKCU only", []string{"windows_registry_uninstall_keys:HKCU"}, false},
		{"HKLM + HKCU", []string{"windows_registry_uninstall_keys:HKLM64", "windows_registry_uninstall_keys:HKCU"}, true},
		{"empty", []string{}, false},
		{"unrelated source", []string{"install_location"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sourcesRequireAdmin(tc.sources); got != tc.want {
				t.Fatalf("sourcesRequireAdmin(%v) = %v, want %v", tc.sources, got, tc.want)
			}
		})
	}
}

// TestReviewEvidenceLabelsAdminRequiredAppsFromHKLMSources verifies the
// preview read model discloses which apps likely need admin before any
// mutation, based on the install source hive (ADR 0028).
func TestReviewEvidenceLabelsAdminRequiredAppsFromHKLMSources(t *testing.T) {
	result := ReviewEvidence(Evidence{
		Applications: []ApplicationEvidence{
			{
				Name:                  "Machine App",
				QuietUninstallCommand: `MsiExec.exe /X{M} /qn`,
				Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
			},
			{
				Name:                  "User App",
				QuietUninstallCommand: `MsiExec.exe /X{U} /qn`,
				Sources:               []string{"windows_registry_uninstall_keys:HKCU"},
			},
			{
				Name:                  "Mixed App",
				QuietUninstallCommand: `MsiExec.exe /X{X} /qn`,
				Sources:               []string{"windows_registry_uninstall_keys:HKLM32", "windows_registry_uninstall_keys:HKCU"},
			},
		},
	})

	if len(result.Applications) != 3 {
		t.Fatalf("applications = %d, want 3", len(result.Applications))
	}
	machine := result.Applications[0]
	user := result.Applications[1]
	mixed := result.Applications[2]
	if machine.Name != "Machine App" || !machine.RequiresAdmin {
		t.Fatalf("Machine App: name=%q requires_admin=%v, want true (HKLM source)", machine.Name, machine.RequiresAdmin)
	}
	if user.Name != "User App" || user.RequiresAdmin {
		t.Fatalf("User App: requires_admin=%v, want false (HKCU-only)", user.RequiresAdmin)
	}
	if mixed.Name != "Mixed App" || !mixed.RequiresAdmin {
		t.Fatalf("Mixed App: requires_admin=%v, want true (has HKLM source)", mixed.RequiresAdmin)
	}
}

// TestRenderPreviewReportDisclosesAdminRequiredApps verifies the human
// preview report groups and discloses admin-required apps before the
// per-application detail, so UAC is expected rather than surprising
// mid-batch (ADR 0028). The disclosure is path-free.
func TestRenderPreviewReportDisclosesAdminRequiredApps(t *testing.T) {
	result := WithReviewSections(Result{
		Status: "preview",
		Applications: []Application{
			{
				Name:          "Machine App",
				PlannedClass:  PlannedClassOfficialUninstaller,
				Evidence:      []string{"windows_registry_uninstall_keys:HKLM64"},
				RequiresAdmin: true,
			},
			{
				Name:          "User App",
				PlannedClass:  PlannedClassOfficialUninstaller,
				Evidence:      []string{"windows_registry_uninstall_keys:HKCU"},
				RequiresAdmin: false,
			},
		},
		Execution: previewOnlyExecution(),
	})

	output := RenderPreviewReport(result)

	// The disclosure section appears before the per-app detail and names the
	// admin-required app.
	for _, want := range []string{
		"Applications likely requiring administrator rights (UAC):",
		"Machine App",
		"Selecting these for --execute may prompt for elevation",
		"requires admin: true (machine-wide install; UAC may be required to uninstall)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	// The User App (HKCU) must not carry a requires-admin label.
	userSection := sectionText(t, output, "User App")
	if strings.Contains(userSection, "requires admin") {
		t.Fatalf("User App section should not contain requires admin:\n%s", userSection)
	}
	// Disclosure appears before the Applications section header detail.
	disclosureIdx := strings.Index(output, "Applications likely requiring administrator rights")
	applicationsIdx := strings.Index(output, "\nApplications\n")
	if disclosureIdx == -1 || applicationsIdx == -1 {
		t.Fatalf("missing disclosure or applications section:\n%s", output)
	}
	if disclosureIdx > applicationsIdx {
		t.Fatalf("admin disclosure must appear before the Applications section:\n%s", output)
	}
}

// TestRenderPreviewReportOmitsAdminDisclosureWhenNoneRequired verifies the
// disclosure is absent when no apps require admin (no noise for HKCU-only
// previews).
func TestRenderPreviewReportOmitsAdminDisclosureWhenNoneRequired(t *testing.T) {
	result := WithReviewSections(Result{
		Status: "preview",
		Applications: []Application{{
			Name:          "User App",
			PlannedClass:  PlannedClassOfficialUninstaller,
			Evidence:      []string{"windows_registry_uninstall_keys:HKCU"},
			RequiresAdmin: false,
		}},
		Execution: previewOnlyExecution(),
	})

	output := RenderPreviewReport(result)
	if strings.Contains(output, "likely requiring administrator rights") {
		t.Fatalf("admin disclosure should be absent when no apps require admin:\n%s", output)
	}
}

// TestRenderExecuteReportShowsElevationOutcome verifies the execute report
// renders the elevation decision so batch and elevation outcomes are clear.
func TestRenderExecuteReportShowsElevationOutcome(t *testing.T) {
	result := ExecuteResult{
		Status: StatusExecuteOK,
		Mode:   ModeExecute,
		Applications: []AppOutcome{{
			Name:          "Admin App",
			RequiresAdmin: true,
			Result:        ResultSkipped,
			SkippedReason: SkipElevationRequiredNotGranted,
		}},
		Elevation: ElevationOutcome{
			Requested: true,
			Granted:   false,
			Reason:    "elevation denied or unavailable; admin-required applications will be skipped",
		},
	}

	output := RenderExecuteReport(result)
	for _, want := range []string{
		"Elevation: not granted (admin-required apps skipped)",
		"requires admin: true (machine-wide install)",
		SkipElevationRequiredNotGranted,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

// TestRenderExecuteReportOmitsElevationWhenNotRequested verifies the execute
// report does not render an elevation line when no admin app was in the batch.
func TestRenderExecuteReportOmitsElevationWhenNotRequested(t *testing.T) {
	result := ExecuteResult{
		Status: StatusExecuteOK,
		Mode:   ModeExecute,
		Applications: []AppOutcome{{
			Name:   "User App",
			Result: ResultUninstalled,
		}},
		Elevation: ElevationOutcome{Requested: false},
	}

	output := RenderExecuteReport(result)
	if strings.Contains(output, "Elevation:") {
		t.Fatalf("elevation line should be absent when not requested:\n%s", output)
	}
}
