package uninstall

import (
	"fmt"
	"strings"
)

const previewReportSectionEntryLimit = 10

// RenderPreviewReport renders the read-only uninstall preview from Result.
func RenderPreviewReport(result Result) string {
	var builder strings.Builder

	builder.WriteString("Foal uninstall\n")
	builder.WriteString("Preview only: read-only review; no changes were made.\n")
	if result.Execution.Reason != "" {
		builder.WriteString("Policy: ")
		builder.WriteString(result.Execution.Reason)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")

	renderAdminRequiredDisclosure(&builder, result.Applications)

	for _, section := range result.ReviewSections {
		switch section.ID {
		case "applications":
			renderApplications(&builder, section.Label, result.Applications)
		case "evidence_sources":
			renderEvidenceSources(&builder, "Evidence sources (reported)", result.EvidenceSources)
		case "possible_leftovers":
			renderPossibleLeftovers(&builder, section.Label, result.PossibleLeftovers, result.Skipped)
		case "shared_state_concerns":
			renderSharedStateConcerns(&builder, section.Label, result.SharedStateConcerns, result.Skipped)
		case "orphaned_residue":
			renderOrphanedResidue(&builder, section.Label, result.OrphanedResidue, result.EvidenceSources)
		case "unknown_state":
			renderUnknownState(&builder, section.Label, result.UnknownState, result.Skipped)
		case "skipped_discovery_sources":
			renderSkippedDiscoverySources(&builder, section.Label, result.Skipped)
		default:
			renderSectionHeader(&builder, section.Label)
			builder.WriteString("  none found\n\n")
		}
	}

	builder.WriteString(fmt.Sprintf("Summary: applications=%d, evidence sources=%d, possible leftovers=%d, shared state concerns=%d, orphaned residue=%d, unknown state=%d, skipped discovery sources=%d\n",
		len(result.Applications),
		len(result.EvidenceSources),
		len(result.PossibleLeftovers),
		len(result.SharedStateConcerns),
		len(result.OrphanedResidue),
		len(result.UnknownState),
		len(result.Skipped),
	))

	return builder.String()
}

func renderApplications(builder *strings.Builder, label string, applications []Application) {
	renderSectionHeader(builder, label)
	if len(applications) == 0 {
		builder.WriteString("  none found\n\n")
		return
	}
	builder.WriteString("  Applications are reported at medium confidence; high-confidence ownership requires multi-source evidence, not implemented yet\n")
	for _, app := range applications[:cappedEntryCount(len(applications))] {
		builder.WriteString("  - ")
		builder.WriteString(valueOrUnknown(app.Name))
		builder.WriteString("\n")
		writeField(builder, "version", app.Version)
		writeField(builder, "publisher", app.Publisher)
		writeField(builder, "plan", plannedClassLabel(app.PlannedClass))
		writeField(builder, "plan reason", app.PlannedReason)
		writeField(builder, "install location", app.InstallLocation)
		writeField(builder, "quiet uninstall command", app.QuietUninstallCommand)
		writeField(builder, "interactive uninstall command", app.InteractiveUninstallCommand)
		writeListField(builder, "evidence", app.Evidence)
		if app.RequiresAdmin {
			writeField(builder, "requires admin", "true (machine-wide install; UAC may be required to uninstall)")
		}
		writeField(builder, "skipped reason", app.SkippedReason)
	}
	writeOmittedLine(builder, len(applications))
	builder.WriteString("\n")
}

// renderAdminRequiredDisclosure writes a grouping disclosure of applications
// that likely require administrator rights, so UAC is expected before
// confirmation rather than surprising mid-batch (ADR 0028). The disclosure
// appears before the per-section detail and is path-free.
func renderAdminRequiredDisclosure(builder *strings.Builder, applications []Application) {
	var adminApps []string
	for _, app := range applications {
		if app.RequiresAdmin {
			adminApps = append(adminApps, app.Name)
		}
	}
	if len(adminApps) == 0 {
		return
	}
	builder.WriteString("Applications likely requiring administrator rights (UAC):\n")
	for _, name := range adminApps {
		builder.WriteString("  - ")
		builder.WriteString(valueOrUnknown(name))
		builder.WriteString("\n")
	}
	builder.WriteString("Selecting these for --execute may prompt for elevation; without it they are skipped with a stable reason.\n\n")
}

func renderEvidenceSources(builder *strings.Builder, label string, sources []EvidenceSource) {
	renderSectionHeader(builder, label)
	var reportedSources []EvidenceSource
	for _, source := range sources {
		if source.Status != "reported" {
			continue
		}
		reportedSources = append(reportedSources, source)
	}
	if len(reportedSources) == 0 {
		builder.WriteString("  none found\n")
		builder.WriteString("\n")
		return
	}
	for _, source := range reportedSources[:cappedEntryCount(len(reportedSources))] {
		builder.WriteString("  - ")
		builder.WriteString(valueOrUnknown(source.Source))
		builder.WriteString("\n")
		writeField(builder, "status", source.Status)
		writeField(builder, "reason", source.Reason)
	}
	writeOmittedLine(builder, len(reportedSources))
	builder.WriteString("\n")
}

func renderPossibleLeftovers(builder *strings.Builder, label string, leftovers []LeftoverCandidate, skipped []SkippedReason) {
	renderSectionHeader(builder, label)
	if len(leftovers) == 0 {
		writeLeftoverEmptyState(builder, skipped)
		return
	}
	for _, leftover := range leftovers[:cappedEntryCount(len(leftovers))] {
		builder.WriteString("  - ")
		builder.WriteString(valueOrUnknown(leftover.Path))
		builder.WriteString("\n")
		writeField(builder, "app", leftover.App)
		writeField(builder, "ownership", leftover.Ownership)
		writeField(builder, "confidence", leftover.Confidence)
		writeField(builder, "reason", leftover.Reason)
	}
	writeOmittedLine(builder, len(leftovers))
	builder.WriteString("\n")
}

func renderSharedStateConcerns(builder *strings.Builder, label string, concerns []SharedStateConcern, skipped []SkippedReason) {
	renderSectionHeader(builder, label)
	if len(concerns) == 0 {
		writeLeftoverEmptyState(builder, skipped)
		return
	}
	for _, concern := range concerns[:cappedEntryCount(len(concerns))] {
		builder.WriteString("  - ")
		builder.WriteString(valueOrUnknown(concern.Path))
		builder.WriteString("\n")
		writeField(builder, "reason", concern.Reason)
	}
	writeOmittedLine(builder, len(concerns))
	builder.WriteString("\n")
}

func renderUnknownState(builder *strings.Builder, label string, unknownState []UnknownStateCandidate, skipped []SkippedReason) {
	renderSectionHeader(builder, label)
	if len(unknownState) == 0 {
		writeLeftoverEmptyState(builder, skipped)
		return
	}
	for _, item := range unknownState[:cappedEntryCount(len(unknownState))] {
		builder.WriteString("  - ")
		builder.WriteString(valueOrUnknown(item.Path))
		builder.WriteString("\n")
		writeField(builder, "reason", item.Reason)
	}
	writeOmittedLine(builder, len(unknownState))
	builder.WriteString("\n")
}

func renderOrphanedResidue(builder *strings.Builder, label string, candidates []OrphanedResidueCandidate, sources []EvidenceSource) {
	renderSectionHeader(builder, label)
	builder.WriteString("  Review only: low-confidence residue clues; not cleanup candidates.\n")
	if len(candidates) == 0 {
		writeOrphanedResidueEmptyState(builder, sources)
		return
	}
	for _, candidate := range candidates[:cappedEntryCount(len(candidates))] {
		builder.WriteString("  - ")
		builder.WriteString(valueOrUnknown(candidate.Path))
		builder.WriteString("\n")
		writeField(builder, "source root", candidate.SourceRoot)
		writeField(builder, "confidence", candidate.Confidence)
		writeField(builder, "reason", candidate.Reason)
	}
	writeOmittedLine(builder, len(candidates))
	builder.WriteString("\n")
}

func renderSkippedDiscoverySources(builder *strings.Builder, label string, skipped []SkippedReason) {
	renderSectionHeader(builder, label)
	if len(skipped) == 0 {
		builder.WriteString("  none found\n\n")
		return
	}
	for _, item := range skipped[:cappedEntryCount(len(skipped))] {
		builder.WriteString("  - ")
		builder.WriteString(valueOrUnknown(item.Source))
		builder.WriteString("\n")
		writeField(builder, "reason", item.Reason)
		builder.WriteString(fmt.Sprintf("    recoverable: %t\n", item.Recoverable))
	}
	writeOmittedLine(builder, len(skipped))
	builder.WriteString("\n")
}

func renderSectionHeader(builder *strings.Builder, label string) {
	builder.WriteString(label)
	builder.WriteString("\n")
}

func writeField(builder *strings.Builder, label, value string) {
	if value == "" {
		return
	}
	builder.WriteString("    ")
	builder.WriteString(label)
	builder.WriteString(": ")
	builder.WriteString(value)
	builder.WriteString("\n")
}

func writeListField(builder *strings.Builder, label string, values []string) {
	if len(values) == 0 {
		return
	}
	builder.WriteString("    ")
	builder.WriteString(label)
	builder.WriteString(": ")
	builder.WriteString(strings.Join(values, ", "))
	builder.WriteString("\n")
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func writeLeftoverEmptyState(builder *strings.Builder, skipped []SkippedReason) {
	if hasSkippedSource(skipped, "known_leftover_locations") {
		builder.WriteString("  Not inspected: leftover discovery is not implemented yet (see Skipped discovery sources)\n\n")
		return
	}
	builder.WriteString("  none found\n\n")
}

func writeOrphanedResidueEmptyState(builder *strings.Builder, sources []EvidenceSource) {
	if evidenceSourceStatus(sources, orphanedResidueSource) == "skipped" {
		builder.WriteString("  Not inspected: orphaned residue discovery was skipped (see Skipped discovery sources)\n\n")
		return
	}
	builder.WriteString("  none found\n\n")
}

func evidenceSourceStatus(sources []EvidenceSource, sourceName string) string {
	for _, source := range sources {
		if source.Source == sourceName {
			return source.Status
		}
	}
	return ""
}

func cappedEntryCount(count int) int {
	if count > previewReportSectionEntryLimit {
		return previewReportSectionEntryLimit
	}
	return count
}

func writeOmittedLine(builder *strings.Builder, count int) {
	omitted := count - previewReportSectionEntryLimit
	if omitted <= 0 {
		return
	}
	builder.WriteString(fmt.Sprintf("  %d omitted. See foal uninstall --json.\n", omitted))
}

// plannedClassLabel maps a stable planned_class JSON value to the domain term
// used in CONTEXT.md and ADRs 0026-0028 for the human preview report. An empty
// class (e.g. a test stub that bypasses ReviewEvidence) renders no field.
func plannedClassLabel(class string) string {
	switch class {
	case PlannedClassOfficialUninstaller:
		return "Official uninstaller invocation"
	case PlannedClassPortableDirectoryRemoval:
		return "Portable directory removal"
	case PlannedClassNotExecutable:
		return "Not executable"
	case PlannedClassHardExclusion:
		return "Uninstall hard exclusion"
	default:
		return ""
	}
}
