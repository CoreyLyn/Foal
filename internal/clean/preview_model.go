package clean

import (
	"fmt"
	"strings"
	"time"
)

type PreviewReadModel struct {
	Title                            string
	Status                           string
	ProtectionRules                  []PreviewProtectionRule
	ProtectionDiagnostics            []ProtectionDiagnostic
	Candidates                       []PreviewCandidate
	Skipped                          []PreviewSkippedItem
	SkippedByDefault                 []PreviewSkippedByDefaultItem
	Opportunities                    []Opportunity
	IncompleteOpportunityInspections []IncompleteOpportunityInspection
	ReviewClues                      []PreviewReviewClue
	ReviewSuggestions                []PreviewReviewSuggestion
	RunningApplicationSkips          []PreviewRunningApplicationSkip
	Errors                           []StructuredIssue
	Notices                          []PreviewNotice
	PotentialSpaceBytes              int64
	CandidateCount                   int
	SkippedCount                     int
	OpportunityCount                 int
	OpportunityObservedBytes         int64
	DetailedListPath                 string
	Summary                          string
}

type PreviewProtectionRule struct {
	ID          string
	Description string
	Path        string
	UserDefined bool
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
	Tool      string
	Label     string
	Command   string
	CachePath string
	NextStep  string
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

type previewReportPresentation struct {
	defaultCandidateLabel string
	skippedLabel          string
	inspectionErrorLabel  string
}

var plainPreviewReportPresentation = previewReportPresentation{
	defaultCandidateLabel: "default candidate",
	skippedLabel:          "skipped",
	inspectionErrorLabel:  "inspection error",
}

const previewReportSectionEntryLimit = 10

const ReviewSuggestionSafetyNote = "Clearing a tool cache while the tool is installing or building can disrupt that operation. Confirm the tool is idle first."
const administratorOnlyCacheBoundaryNotice = "Permission boundary: administrator-only caches such as SoftwareDistribution and Delivery Optimization are excluded from Opportunity discovery. Foal will not request elevation automatically."

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

	protectionRules := make([]PreviewProtectionRule, 0, len(result.DefaultRuleCatalog)+len(result.ProtectionRules))
	for _, rule := range result.DefaultRuleCatalog {
		if !rule.DefaultEnabled {
			continue
		}
		protectionRules = append(protectionRules, PreviewProtectionRule{
			ID:          rule.ID,
			Description: rule.Description,
		})
	}
	for _, rule := range result.ProtectionRules {
		protectionRules = append(protectionRules, PreviewProtectionRule{
			Path:        rule.Path,
			UserDefined: true,
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

	notices := []PreviewNotice{{
		Kind:    "permission_boundary",
		Message: administratorOnlyCacheBoundaryNotice,
	}}
	if hasPermissionBoundary {
		notices = append(notices, PreviewNotice{
			Kind:    "permission_boundary",
			Message: "Permission boundary: Foal skipped protected or administrator-only locations during preview. Review the skipped entries as boundaries; Foal will not request elevation automatically.",
		})
	}

	reviewSuggestions := make([]PreviewReviewSuggestion, 0, len(result.ReviewSuggestions))
	for _, suggestion := range result.ReviewSuggestions {
		reviewSuggestions = append(reviewSuggestions, PreviewReviewSuggestion{
			Tool:      suggestion.Tool,
			Label:     suggestion.Label,
			Command:   suggestion.Command,
			CachePath: suggestion.CachePath,
		})
	}
	opportunities := append([]Opportunity(nil), result.Opportunities...)
	for index := range opportunities {
		opportunities[index].Category = normalizedOpportunityCategory(opportunities[index].Category)
	}

	return PreviewReadModel{
		Title:                            "Foal clean",
		Status:                           "preview_only",
		ProtectionRules:                  protectionRules,
		ProtectionDiagnostics:            append([]ProtectionDiagnostic(nil), result.ProtectionDiagnostics...),
		Candidates:                       candidates,
		Skipped:                          skippedItems,
		Opportunities:                    opportunities,
		IncompleteOpportunityInspections: append([]IncompleteOpportunityInspection(nil), result.IncompleteOpportunityInspections...),
		ReviewSuggestions:                reviewSuggestions,
		Errors:                           append([]StructuredIssue(nil), result.Errors...),
		Notices:                          notices,
		PotentialSpaceBytes:              potentialSpace,
		CandidateCount:                   len(candidates),
		SkippedCount:                     len(result.Skipped),
		OpportunityCount:                 result.Totals.OpportunityCount,
		OpportunityObservedBytes:         result.Totals.OpportunityObservedBytes,
		DetailedListPath:                 result.DetailedListPath,
		Summary:                          "Dry-run summary: No changes were made. Re-run with foal clean --execute to move these default candidates to the Recycle Bin.",
	}
}

func isPermissionBoundaryCode(code string) bool {
	return code == "protected_path" || code == "permission_denied"
}

func RenderPreviewReport(model PreviewReadModel) string {
	return renderPreviewReport(model, plainPreviewReportPresentation)
}

func RenderFailureReport(result Result) string {
	var builder strings.Builder
	builder.WriteString("Foal clean\n")
	builder.WriteString("Clean stopped before candidate scanning because required configuration could not be loaded.\n")
	if len(result.ProtectionDiagnostics) > 0 {
		builder.WriteString("\nProtection diagnostics\n")
		for _, diagnostic := range result.ProtectionDiagnostics {
			builder.WriteString(fmt.Sprintf("  %s (source: %s", diagnostic.Code, diagnostic.Source))
			if diagnostic.Line > 0 {
				builder.WriteString(fmt.Sprintf(", line %d", diagnostic.Line))
			}
			builder.WriteString(fmt.Sprintf(", recoverable: %t)\n", diagnostic.Recoverable))
		}
	}
	builder.WriteString("\nConfiguration errors\n")
	for _, issue := range result.Errors {
		builder.WriteString(fmt.Sprintf("  %s (path: %s, recoverable: %t)\n", issue.Code, issue.Path, issue.Recoverable))
		if issue.Message != "" {
			builder.WriteString(fmt.Sprintf("    %s\n", issue.Message))
		}
	}
	return builder.String()
}

func renderPreviewReport(model PreviewReadModel, presentation previewReportPresentation) string {
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
			if rule.UserDefined {
				builder.WriteString(fmt.Sprintf("  %s (user-defined Protection rule)\n", rule.Path))
				continue
			}
			builder.WriteString(fmt.Sprintf("  %s: %s\n", rule.ID, rule.Description))
		}
	}

	if len(model.ProtectionDiagnostics) > 0 {
		builder.WriteString("\nProtection diagnostics\n")
		for _, diagnostic := range model.ProtectionDiagnostics {
			builder.WriteString(fmt.Sprintf("  %s (source: %s", diagnostic.Code, diagnostic.Source))
			if diagnostic.Line > 0 {
				builder.WriteString(fmt.Sprintf(", line %d", diagnostic.Line))
			}
			builder.WriteString(fmt.Sprintf(", recoverable: %t)\n", diagnostic.Recoverable))
			if diagnostic.Message != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", diagnostic.Message))
			}
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
		for _, candidate := range model.Candidates[:cappedEntryCount(len(model.Candidates))] {
			builder.WriteString(fmt.Sprintf("  %s (%s, %srule: %s, planned action: Recycle Bin)\n",
				candidate.Path, formatBytes(candidate.Bytes), statusLabel(presentation.defaultCandidateLabel), candidate.Rule))
		}
		writeOmittedLine(&builder, len(model.Candidates), model.DetailedListPath)
	}

	builder.WriteString("\nSkipped items\n")
	if len(model.Skipped) == 0 {
		builder.WriteString("  No skipped cleanup paths reported.\n")
	} else {
		for _, skipped := range model.Skipped[:cappedEntryCount(len(model.Skipped))] {
			builder.WriteString(fmt.Sprintf("  %s (%srule: %s, reason: %s, recoverable: %t)\n",
				skipped.Path, statusLabel(presentation.skippedLabel), skipped.Rule, skipped.Reason.Code, skipped.Reason.Recoverable))
			if skipped.Reason.Message != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", skipped.Reason.Message))
			}
		}
		writeOmittedLine(&builder, len(model.Skipped), model.DetailedListPath)
	}

	if len(model.Opportunities) > 0 || len(model.SkippedByDefault) > 0 {
		builder.WriteString("\nSkipped by default\n")
		if len(model.Opportunities) > 0 {
			builder.WriteString(fmt.Sprintf("  Opportunities: %d, observed bytes: %s (not counted as Potential space)\n",
				model.OpportunityCount, formatBytes(model.OpportunityObservedBytes)))
			for _, opportunity := range model.Opportunities[:cappedEntryCount(len(model.Opportunities))] {
				builder.WriteString(fmt.Sprintf("  %s (%s, category: %s",
					opportunity.Path, formatBytes(opportunity.Bytes), normalizedOpportunityCategory(opportunity.Category)))
				if normalizedOpportunityCategory(opportunity.Category) == OpportunityCategoryUserTemp {
					builder.WriteString(fmt.Sprintf(", latest modified: %s, idle days: %d",
						opportunity.LatestModifiedAt.UTC().Format(time.RFC3339), opportunity.IdleDays))
				}
				builder.WriteString(fmt.Sprintf(", status: %s, reason: %s, not counted as Potential space)\n",
					opportunity.Status, opportunity.Reason))
			}
			writeOmittedLine(&builder, len(model.Opportunities), model.DetailedListPath)
		}
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
		builder.WriteString(fmt.Sprintf("  %s\n", ReviewSuggestionSafetyNote))
		for _, suggestion := range model.ReviewSuggestions {
			builder.WriteString(fmt.Sprintf("  %s\n", suggestion.Label))
			if suggestion.Command != "" {
				builder.WriteString(fmt.Sprintf("    Command: %s\n", suggestion.Command))
			}
			if suggestion.CachePath != "" {
				builder.WriteString(fmt.Sprintf("    Cache: %s\n", suggestion.CachePath))
			}
			if suggestion.Command == "" && suggestion.NextStep != "" {
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
		for _, err := range model.Errors[:cappedEntryCount(len(model.Errors))] {
			builder.WriteString(fmt.Sprintf("  %s (%srule: %s, error: %s, recoverable: %t)\n",
				err.Path, statusLabel(presentation.inspectionErrorLabel), err.Rule, err.Code, err.Recoverable))
			if err.Message != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", err.Message))
			}
		}
		writeOmittedLine(&builder, len(model.Errors), model.DetailedListPath)
	}

	builder.WriteString("\n")
	builder.WriteString(fmt.Sprintf("Candidates: %d, skipped: %d, errors: %d.\n", model.CandidateCount, model.SkippedCount, len(model.Errors)))
	builder.WriteString(model.Summary)
	builder.WriteString("\n")
	return builder.String()
}

func cappedEntryCount(count int) int {
	if count > previewReportSectionEntryLimit {
		return previewReportSectionEntryLimit
	}
	return count
}

func writeOmittedLine(builder *strings.Builder, count int, detailedListPath string) {
	omitted := count - previewReportSectionEntryLimit
	if omitted <= 0 {
		return
	}
	builder.WriteString(fmt.Sprintf("  %d omitted.", omitted))
	if detailedListPath != "" {
		builder.WriteString(fmt.Sprintf(" See detailed candidate list for full path detail: %s", detailedListPath))
	}
	builder.WriteString("\n")
}

func statusLabel(label string) string {
	if label == "" {
		return ""
	}
	return fmt.Sprintf("status: %s, ", label)
}

func formatBytes(bytes int64) string {
	return fmt.Sprintf("%d bytes", bytes)
}
