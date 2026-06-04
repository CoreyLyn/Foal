package clean

import (
	"fmt"
	"strings"
)

type PreviewReadModel struct {
	Title               string
	Status              string
	ProtectionRules     []PreviewProtectionRule
	Candidates          []PreviewCandidate
	PotentialSpaceBytes int64
	CandidateCount      int
	SkippedCount        int
	Summary             string
}

type PreviewProtectionRule struct {
	ID          string
	Description string
}

type PreviewCandidate struct {
	Path          string
	Bytes         int64
	Rule          string
	PlannedAction string
}

func NewPreviewReadModel(result Result) PreviewReadModel {
	candidates := make([]PreviewCandidate, 0, len(result.Candidates))
	var potentialSpace int64
	for _, candidate := range result.Candidates {
		potentialSpace += candidate.Bytes
		candidates = append(candidates, PreviewCandidate{
			Path:          candidate.Path,
			Bytes:         candidate.Bytes,
			Rule:          candidate.Rule,
			PlannedAction: candidate.PlannedAction,
		})
	}

	protectionRules := make([]PreviewProtectionRule, 0, len(result.DefaultRuleCatalog))
	for _, rule := range result.DefaultRuleCatalog {
		if !rule.DefaultEnabled {
			continue
		}
		protectionRules = append(protectionRules, PreviewProtectionRule{
			ID:          rule.ID,
			Description: rule.Description,
		})
	}

	return PreviewReadModel{
		Title:               "Foal clean",
		Status:              "preview_only",
		ProtectionRules:     protectionRules,
		Candidates:          candidates,
		PotentialSpaceBytes: potentialSpace,
		CandidateCount:      len(candidates),
		SkippedCount:        len(result.Skipped),
		Summary:             "Dry-run summary: No changes were made. Re-run with foal clean --execute to move these default candidates to the Recycle Bin.",
	}
}

func RenderPreviewReport(model PreviewReadModel) string {
	var builder strings.Builder
	builder.WriteString(model.Title)
	builder.WriteString("\n")
	builder.WriteString("Preview only: Foal inspected default cleanup candidates and did not change files.\n")
	builder.WriteString(fmt.Sprintf("Potential space: %s\n", formatBytes(model.PotentialSpaceBytes)))
	builder.WriteString("\nProtection rules\n")
	if len(model.ProtectionRules) == 0 {
		builder.WriteString("  No default-enabled protection rules were reported.\n")
	} else {
		for _, rule := range model.ProtectionRules {
			builder.WriteString(fmt.Sprintf("  %s: %s\n", rule.ID, rule.Description))
		}
	}

	builder.WriteString("\nDefault candidates\n")
	if len(model.Candidates) == 0 {
		builder.WriteString("  No default candidates found.\n")
	} else {
		for _, candidate := range model.Candidates {
			builder.WriteString(fmt.Sprintf("  %s (%s, rule: %s, planned action: Recycle Bin)\n",
				candidate.Path, formatBytes(candidate.Bytes), candidate.Rule))
		}
	}

	builder.WriteString("\n")
	builder.WriteString(fmt.Sprintf("Candidates: %d, skipped: %d.\n", model.CandidateCount, model.SkippedCount))
	builder.WriteString(model.Summary)
	builder.WriteString("\n")
	return builder.String()
}

func formatBytes(bytes int64) string {
	return fmt.Sprintf("%d bytes", bytes)
}
