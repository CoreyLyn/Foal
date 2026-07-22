package analyze

import (
	"fmt"
	"strings"
)

// RenderHumanReport renders a human-readable directory insight report from Result.
// It never claims complete tree size when Status is StatusIncomplete, and
// includes guarded purge handoff copy only when a top child has classification
// project_artifact_clue and the analysis root independently passes Purge root
// validation (ValidateUserScanRoot). Volume/system roots never get unusable hints.
func RenderHumanReport(result Result) string {
	var b strings.Builder
	b.WriteString("Foal analyze\n")
	b.WriteString(fmt.Sprintf("Root: %s\n", result.Root))
	if result.Status == StatusIncomplete {
		b.WriteString("Status: incomplete (partial results only, no full tree size)\n")
	} else {
		b.WriteString("Status: ok\n")
	}
	b.WriteString(fmt.Sprintf("Totals: %d bytes, %d files, %d directories\n",
		result.Totals.Bytes, result.Totals.FileCount, result.Totals.DirectoryCount))
	b.WriteString(fmt.Sprintf("Skipped: %d\n", len(result.Skipped)))

	if len(result.TopChildren) > 0 {
		b.WriteString("\nTop children by size:\n")
		for _, child := range result.TopChildren {
			var classification string
			if child.Classification != "" {
				classification = fmt.Sprintf(" (%s)", child.Classification)
			}
			b.WriteString(fmt.Sprintf("  %-12s %10d  %s%s\n", child.Kind, child.Bytes, child.Name, classification))
		}
	}

	// Guarded copy-only Purge handoff (never launches Purge).
	hasArtifactClue := false
	for _, child := range result.TopChildren {
		if child.Classification == ClassificationProjectArtifactClue {
			hasArtifactClue = true
			break
		}
	}
	if handoff := FormatPurgeHandoffCopy(result.Root, hasArtifactClue); handoff != "" {
		b.WriteString("\n")
		b.WriteString(handoff)
	}

	return b.String()
}

// escapeForShell does minimal safe escaping for shell example copy.
// It does NOT guarantee safety for untrusted input, just avoids obvious breakage.
func escapeForShell(path string) string {
	// For Windows paths, wrap in quotes if contains spaces.
	if strings.Contains(path, " ") {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return path
}
