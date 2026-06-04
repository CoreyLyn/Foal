package clean

import (
	"fmt"
	"strings"
)

type PreviewReadModel struct {
	Title                   string
	Status                  string
	ProtectionRules         []PreviewProtectionRule
	Candidates              []PreviewCandidate
	Skipped                 []PreviewSkippedItem
	SkippedByDefault        []PreviewSkippedByDefaultItem
	ReviewClues             []PreviewReviewClue
	ReviewSuggestions       []PreviewReviewSuggestion
	RunningApplicationSkips []PreviewRunningApplicationSkip
	Errors                  []StructuredIssue
	Notices                 []PreviewNotice
	PotentialSpaceBytes     int64
	CandidateCount          int
	SkippedCount            int
	DetailedListPath        string
	Summary                 string
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

type PreviewSkippedItem struct {
	Path   string
	Bytes  int64
	Rule   string
	Reason StructuredIssue
}

type PreviewSkippedByDefaultItem struct {
	Name   string
	Path   string
	Bytes  int64
	Reason string
}

type PreviewReviewClue struct {
	Name    string
	Path    string
	Details string
}

type PreviewReviewSuggestion struct {
	Label    string
	NextStep string
}

type PreviewRunningApplicationSkip struct {
	Name        string
	Path        string
	Application string
	Reason      string
}

type PreviewNotice struct {
	Kind    string
	Message string
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

	skippedItems := make([]PreviewSkippedItem, 0, len(result.Skipped))
	hasPermissionBoundary := false
	for _, skipped := range result.Skipped {
		if isPermissionBoundaryCode(skipped.Reason.Code) {
			hasPermissionBoundary = true
		}
		skippedItems = append(skippedItems, PreviewSkippedItem{
			Path:   skipped.Path,
			Bytes:  skipped.Bytes,
			Rule:   skipped.Rule,
			Reason: skipped.Reason,
		})
	}
	for _, err := range result.Errors {
		if isPermissionBoundaryCode(err.Code) {
			hasPermissionBoundary = true
		}
	}

	notices := []PreviewNotice{}
	if hasPermissionBoundary {
		notices = append(notices, PreviewNotice{
			Kind:    "permission_boundary",
			Message: "Permission boundary: Foal skipped protected or administrator-only locations during preview. Review the skipped entries as boundaries; Foal will not request elevation automatically.",
		})
	}

	return PreviewReadModel{
		Title:               "Foal clean",
		Status:              "preview_only",
		ProtectionRules:     protectionRules,
		Candidates:          candidates,
		Skipped:             skippedItems,
		Errors:              append([]StructuredIssue(nil), result.Errors...),
		Notices:             notices,
		PotentialSpaceBytes: potentialSpace,
		CandidateCount:      len(candidates),
		SkippedCount:        len(result.Skipped),
		DetailedListPath:    result.DetailedListPath,
		Summary:             "Dry-run summary: No changes were made. Re-run with foal clean --execute to move these default candidates to the Recycle Bin.",
	}
}

func isPermissionBoundaryCode(code string) bool {
	return code == "protected_path" || code == "permission_denied"
}

func RenderPreviewReport(model PreviewReadModel) string {
	var builder strings.Builder
	builder.WriteString(model.Title)
	builder.WriteString("\n")
	builder.WriteString("Preview only: Foal inspected default cleanup candidates and did not change files.\n")
	builder.WriteString(fmt.Sprintf("Potential space: %s\n", formatBytes(model.PotentialSpaceBytes)))
	if model.DetailedListPath != "" {
		builder.WriteString(fmt.Sprintf("Detailed candidate list: %s\n", model.DetailedListPath))
	}
	builder.WriteString("\nProtection rules\n")
	if len(model.ProtectionRules) == 0 {
		builder.WriteString("  No default-enabled protection rules were reported.\n")
	} else {
		for _, rule := range model.ProtectionRules {
			builder.WriteString(fmt.Sprintf("  %s: %s\n", rule.ID, rule.Description))
		}
	}

	if len(model.Notices) > 0 {
		builder.WriteString("\nNotices\n")
		for _, notice := range model.Notices {
			builder.WriteString(fmt.Sprintf("  %s\n", notice.Message))
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

	builder.WriteString("\nSkipped items\n")
	if len(model.Skipped) == 0 {
		builder.WriteString("  No skipped cleanup paths reported.\n")
	} else {
		for _, skipped := range model.Skipped {
			builder.WriteString(fmt.Sprintf("  %s (rule: %s, reason: %s, recoverable: %t)\n",
				skipped.Path, skipped.Rule, skipped.Reason.Code, skipped.Reason.Recoverable))
			if skipped.Reason.Message != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", skipped.Reason.Message))
			}
		}
	}

	if len(model.SkippedByDefault) > 0 {
		builder.WriteString("\nSkipped by default\n")
		for _, skipped := range model.SkippedByDefault {
			builder.WriteString(fmt.Sprintf("  %s", skipped.Name))
			if skipped.Path != "" {
				builder.WriteString(fmt.Sprintf(" - %s", skipped.Path))
			}
			if skipped.Bytes > 0 {
				builder.WriteString(fmt.Sprintf(" (%s, not counted as Potential space)", formatBytes(skipped.Bytes)))
			}
			builder.WriteString("\n")
			if skipped.Reason != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", skipped.Reason))
			}
		}
	}

	if len(model.ReviewClues) > 0 {
		builder.WriteString("\nReview clues\n")
		for _, clue := range model.ReviewClues {
			builder.WriteString(fmt.Sprintf("  %s", clue.Name))
			if clue.Path != "" {
				builder.WriteString(fmt.Sprintf(" - %s", clue.Path))
			}
			builder.WriteString("\n")
			if clue.Details != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", clue.Details))
			}
		}
	}

	if len(model.ReviewSuggestions) > 0 {
		builder.WriteString("\nReview suggestions\n")
		for _, suggestion := range model.ReviewSuggestions {
			builder.WriteString(fmt.Sprintf("  %s\n", suggestion.Label))
			if suggestion.NextStep != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", suggestion.NextStep))
			}
		}
	}

	if len(model.RunningApplicationSkips) > 0 {
		builder.WriteString("\nRunning application skips\n")
		for _, skipped := range model.RunningApplicationSkips {
			builder.WriteString(fmt.Sprintf("  %s", skipped.Name))
			if skipped.Application != "" {
				builder.WriteString(fmt.Sprintf(" (%s)", skipped.Application))
			}
			if skipped.Path != "" {
				builder.WriteString(fmt.Sprintf(" - %s", skipped.Path))
			}
			builder.WriteString("\n")
			if skipped.Reason != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", skipped.Reason))
			}
		}
	}

	builder.WriteString("\nInspection errors\n")
	if len(model.Errors) == 0 {
		builder.WriteString("  No recoverable inspection errors reported.\n")
	} else {
		for _, err := range model.Errors {
			builder.WriteString(fmt.Sprintf("  %s (rule: %s, error: %s, recoverable: %t)\n",
				err.Path, err.Rule, err.Code, err.Recoverable))
			if err.Message != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", err.Message))
			}
		}
	}

	builder.WriteString("\n")
	builder.WriteString(fmt.Sprintf("Candidates: %d, skipped: %d, errors: %d.\n", model.CandidateCount, model.SkippedCount, len(model.Errors)))
	builder.WriteString(model.Summary)
	builder.WriteString("\n")
	return builder.String()
}

func formatBytes(bytes int64) string {
	return fmt.Sprintf("%d bytes", bytes)
}
