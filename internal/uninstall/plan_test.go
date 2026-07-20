package uninstall

import "testing"

func TestClassifyApplicationPlanOfficialUninstallerWhenQuietCommandPresent(t *testing.T) {
	class, reason := classifyApplicationPlan(ApplicationEvidence{
		Name:                  "Quiet App",
		QuietUninstallCommand: `MsiExec.exe /X{GUID} /qn`,
	})

	if class != PlannedClassOfficialUninstaller {
		t.Fatalf("class = %q, want %q", class, PlannedClassOfficialUninstaller)
	}
	if reason == "" {
		t.Fatalf("reason = %q, want non-empty domain-aligned reason", reason)
	}
}

func TestClassifyApplicationPlanOfficialUninstallerWhenOnlyInteractiveCommandPresent(t *testing.T) {
	class, _ := classifyApplicationPlan(ApplicationEvidence{
		Name:                        "Interactive App",
		InteractiveUninstallCommand: `"C:\Program Files\App\uninstall.exe"`,
	})

	if class != PlannedClassOfficialUninstaller {
		t.Fatalf("class = %q, want %q (interactive command alone qualifies)", class, PlannedClassOfficialUninstaller)
	}
}

func TestClassifyApplicationPlanPortableDirectoryRemovalWhenNoCommandButInstallLocationPresent(t *testing.T) {
	class, reason := classifyApplicationPlan(ApplicationEvidence{
		Name:            "Portable App",
		InstallLocation: `C:\Apps\PortableApp`,
	})

	if class != PlannedClassPortableDirectoryRemoval {
		t.Fatalf("class = %q, want %q", class, PlannedClassPortableDirectoryRemoval)
	}
	if reason == "" {
		t.Fatalf("reason = %q, want non-empty domain-aligned reason", reason)
	}
}

func TestClassifyApplicationPlanNotExecutableWhenNoCommandAndNoInstallLocation(t *testing.T) {
	class, reason := classifyApplicationPlan(ApplicationEvidence{
		Name: "Bare App",
	})

	if class != PlannedClassNotExecutable {
		t.Fatalf("class = %q, want %q", class, PlannedClassNotExecutable)
	}
	if reason == "" {
		t.Fatalf("reason = %q, want non-empty domain-aligned reason", reason)
	}
}

func TestClassifyApplicationPlanHardExclusionTakesPrecedenceOverCommands(t *testing.T) {
	class, reason := classifyApplicationPlan(ApplicationEvidence{
		Name:                  "Foal",
		QuietUninstallCommand: `MsiExec.exe /X{GUID} /qn`,
		InstallLocation:       `C:\Program Files\Foal`,
	})

	if class != PlannedClassHardExclusion {
		t.Fatalf("class = %q, want %q (hard exclusion outranks available commands)", class, PlannedClassHardExclusion)
	}
	if reason == "" {
		t.Fatalf("reason = %q, want non-empty domain-aligned reason", reason)
	}
}

func TestIsUninstallHardExclusionMatchesFoalCaseInsensitively(t *testing.T) {
	for _, name := range []string{"Foal", "foal", "FOAL", "fOaL"} {
		if !isUninstallHardExclusion(name) {
			t.Fatalf("isUninstallHardExclusion(%q) = false, want true", name)
		}
	}
}

func TestIsUninstallHardExclusionDoesNotMatchSubstringOrUnrelatedNames(t *testing.T) {
	for _, name := range []string{"Foal Cleaner", "MyFoal", "FoalHelper", "Example App", ""} {
		if isUninstallHardExclusion(name) {
			t.Fatalf("isUninstallHardExclusion(%q) = true, want false (exact match only)", name)
		}
	}
}
