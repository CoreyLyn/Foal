package analyze

import (
	"path/filepath"
	"strings"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// ClassificationProjectArtifactClue is the stable JSON/classification token for a
// direct-child rebuildable project directory name match.
const ClassificationProjectArtifactClue = "project_artifact_clue"

// PresentationArtifactLabel is the compact TUI label for a project artifact clue.
// It is presentation-only; JSON continues to use ClassificationProjectArtifactClue.
const PresentationArtifactLabel = "artifact"

// HasProjectArtifactClue reports whether any classified child carries the
// project_artifact_clue token. Callers pass top children (CLI) or browse children (TUI).
func HasProjectArtifactClue(classifications ...string) bool {
	for _, c := range classifications {
		if c == ClassificationProjectArtifactClue {
			return true
		}
	}
	return false
}

// ShouldOfferPurgeHandoff reports whether Analyze may show copy-only Purge guidance
// for the current measurement/browse root. The root must independently pass Purge's
// ValidateUserScanRoot policy and at least one direct child must carry a project
// artifact clue. Volume roots, Windows-managed trees, and the user profile root never
// receive an unusable Purge hint. This never launches Purge or authorizes deletion.
func ShouldOfferPurgeHandoff(root string, hasDirectArtifactClue bool) bool {
	if !hasDirectArtifactClue {
		return false
	}
	if strings.TrimSpace(root) == "" {
		return false
	}
	// Normalize for policy checks; relative roots are not valid purge roots.
	clean := root
	if abs, err := filepath.Abs(root); err == nil {
		clean = abs
	}
	if _, ok := pathsafe.ValidateUserScanRoot(clean); !ok {
		return false
	}
	return true
}

// FormatPurgeHandoffCopy returns the read-only next-step sentence for an allowed root.
// Empty when handoff must not be offered. Copy-only: never executes or launches purge.
func FormatPurgeHandoffCopy(root string, hasDirectArtifactClue bool) string {
	if !ShouldOfferPurgeHandoff(root, hasDirectArtifactClue) {
		return ""
	}
	return "Next step: run `foal purge " + escapeForShell(root) +
		"` for explicit-root preview and permanent reclaim of project artifacts.\n"
}

// CompactClassificationLabel maps a stable classification token to a short TUI label.
// Unknown or empty classifications yield empty (no invented cleanup language).
func CompactClassificationLabel(classification string) string {
	if classification == ClassificationProjectArtifactClue {
		return PresentationArtifactLabel
	}
	return ""
}
