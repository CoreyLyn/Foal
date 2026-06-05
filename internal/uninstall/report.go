package uninstall

import (
	"fmt"
	"strings"
)

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

	for _, section := range result.ReviewSections {
		switch section.ID {
		case "applications":
			renderApplications(&builder, section.Label, result.Applications)
		case "evidence_sources":
			renderEvidenceSources(&builder, "Evidence sources (reported)", result.EvidenceSources)
		case "possible_leftovers":
			renderPossibleLeftovers(&builder, section.Label, result.PossibleLeftovers)
		case "shared_state_concerns":
			renderSharedStateConcerns(&builder, section.Label, result.SharedStateConcerns)
		case "unknown_state":
			renderUnknownState(&builder, section.Label, result.UnknownState)
		case "skipped_discovery_sources":
			renderSkippedDiscoverySources(&builder, section.Label, result.Skipped)
		default:
			renderSectionHeader(&builder, section.Label)
			builder.WriteString("  None reported.\n\n")
		}
	}

	builder.WriteString(fmt.Sprintf("Summary: applications=%d, evidence sources=%d, possible leftovers=%d, shared state concerns=%d, unknown state=%d, skipped discovery sources=%d\n",
		len(result.Applications),
		len(result.EvidenceSources),
		len(result.PossibleLeftovers),
		len(result.SharedStateConcerns),
		len(result.UnknownState),
		len(result.Skipped),
	))

	return builder.String()
}

func renderApplications(builder *strings.Builder, label string, applications []Application) {
	renderSectionHeader(builder, label)
	if len(applications) == 0 {
		builder.WriteString("  None reported.\n\n")
		return
	}
	for _, app := range applications {
		builder.WriteString("  - ")
		builder.WriteString(valueOrUnknown(app.Name))
		builder.WriteString("\n")
		writeField(builder, "version", app.Version)
		writeField(builder, "publisher", app.Publisher)
		writeField(builder, "confidence", app.Confidence)
		writeField(builder, "ownership", app.Ownership)
		writeListField(builder, "evidence", app.Evidence)
		writeField(builder, "skipped reason", app.SkippedReason)
	}
	builder.WriteString("\n")
}

func renderEvidenceSources(builder *strings.Builder, label string, sources []EvidenceSource) {
	renderSectionHeader(builder, label)
	reported := false
	for _, source := range sources {
		if source.Status != "reported" {
			continue
		}
		reported = true
		builder.WriteString("  - ")
		builder.WriteString(valueOrUnknown(source.Source))
		builder.WriteString("\n")
		writeField(builder, "status", source.Status)
		writeField(builder, "reason", source.Reason)
	}
	if !reported {
		builder.WriteString("  None reported.\n")
	}
	builder.WriteString("\n")
}

func renderPossibleLeftovers(builder *strings.Builder, label string, leftovers []LeftoverCandidate) {
	renderSectionHeader(builder, label)
	if len(leftovers) == 0 {
		builder.WriteString("  None reported.\n\n")
		return
	}
	for _, leftover := range leftovers {
		builder.WriteString("  - ")
		builder.WriteString(valueOrUnknown(leftover.Path))
		builder.WriteString("\n")
		writeField(builder, "app", leftover.App)
		writeField(builder, "ownership", leftover.Ownership)
		writeField(builder, "confidence", leftover.Confidence)
		writeField(builder, "reason", leftover.Reason)
	}
	builder.WriteString("\n")
}

func renderSharedStateConcerns(builder *strings.Builder, label string, concerns []SharedStateConcern) {
	renderSectionHeader(builder, label)
	if len(concerns) == 0 {
		builder.WriteString("  None reported.\n\n")
		return
	}
	for _, concern := range concerns {
		builder.WriteString("  - ")
		builder.WriteString(valueOrUnknown(concern.Path))
		builder.WriteString("\n")
		writeField(builder, "reason", concern.Reason)
	}
	builder.WriteString("\n")
}

func renderUnknownState(builder *strings.Builder, label string, unknownState []UnknownStateCandidate) {
	renderSectionHeader(builder, label)
	if len(unknownState) == 0 {
		builder.WriteString("  None reported.\n\n")
		return
	}
	for _, item := range unknownState {
		builder.WriteString("  - ")
		builder.WriteString(valueOrUnknown(item.Path))
		builder.WriteString("\n")
		writeField(builder, "reason", item.Reason)
	}
	builder.WriteString("\n")
}

func renderSkippedDiscoverySources(builder *strings.Builder, label string, skipped []SkippedReason) {
	renderSectionHeader(builder, label)
	if len(skipped) == 0 {
		builder.WriteString("  None reported.\n\n")
		return
	}
	for _, item := range skipped {
		builder.WriteString("  - ")
		builder.WriteString(valueOrUnknown(item.Source))
		builder.WriteString("\n")
		writeField(builder, "reason", item.Reason)
		builder.WriteString(fmt.Sprintf("    recoverable: %t\n", item.Recoverable))
	}
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
