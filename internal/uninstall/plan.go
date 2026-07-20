package uninstall

import "strings"

// Planned execution class values for traditional desktop applications. These
// stable JSON values describe how Foal would execute an uninstall for a given
// application; preview output reports them without performing any mutation.
// Vocabulary mirrors CONTEXT.md and ADRs 0026-0028.
const (
	// PlannedClassOfficialUninstaller is the class for an application that
	// advertises a registry uninstall command (quiet preferred, then
	// interactive). See "Official uninstaller invocation" in CONTEXT.md.
	PlannedClassOfficialUninstaller = "official_uninstaller"
	// PlannedClassPortableDirectoryRemoval is the exceptional class used when
	// no uninstall command exists but a trusted install location is known.
	// Execution requires explicit permanent-deletion authorization; the plan
	// only reports the candidate class. See "Portable directory removal".
	PlannedClassPortableDirectoryRemoval = "portable_directory_removal"
	// PlannedClassNotExecutable is the class for an application Foal discovers
	// but cannot plan any uninstall execution for (no command, no install
	// location).
	PlannedClassNotExecutable = "not_executable"
	// PlannedClassHardExclusion is the class for an application Foal never
	// offers for Uninstall execution, including Foal itself and the small
	// fixed denylist. See "Uninstall hard exclusion" in CONTEXT.md.
	PlannedClassHardExclusion = "hard_exclusion"
)

// uninstallHardExclusionDenylist is the small fixed set of application display
// names Foal never offers for Uninstall execution. It is intentionally minimal
// and case-insensitively matched by exact display name; system components are
// already hidden by registry discovery filters (SystemComponent / ParentKeyName).
// Add entries here only when Foal must refuse a visible app by name.
var uninstallHardExclusionDenylist = []string{
	"Foal", // self-referential: never offer Foal for uninstall
}

// isUninstallHardExclusion reports whether an application display name is a
// hard exclusion (exact, case-insensitive match against the denylist). A
// substring match is intentionally not accepted so unrelated apps sharing a
// token are not silently excluded.
func isUninstallHardExclusion(name string) bool {
	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		return false
	}
	for _, entry := range uninstallHardExclusionDenylist {
		if strings.ToLower(entry) == target {
			return true
		}
	}
	return false
}

// classifyApplicationPlan determines the planned execution class for a
// traditional desktop application from its discovery evidence, without
// performing any mutation. Hard exclusion takes precedence over available
// commands and install location. The returned reason is a stable, human-readable
// explanation aligned with the domain vocabulary.
func classifyApplicationPlan(app ApplicationEvidence) (class, reason string) {
	if isUninstallHardExclusion(app.Name) {
		return PlannedClassHardExclusion, "Foal never offers this application for Uninstall execution"
	}
	if app.QuietUninstallCommand != "" || app.InteractiveUninstallCommand != "" {
		return PlannedClassOfficialUninstaller, "registry-advertised uninstall command is available"
	}
	if app.InstallLocation != "" {
		return PlannedClassPortableDirectoryRemoval, "no uninstall command; install location is present"
	}
	return PlannedClassNotExecutable, "no uninstall command and no install location"
}
