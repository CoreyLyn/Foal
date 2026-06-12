package clean

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/delete"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/history"
)

const plannedRecycleBinAction = "move_to_recycle_bin"

type Options struct {
	Rules                         []Rule
	Validator                     pathsafe.Validator
	RecycleBinAdapter             delete.Adapter
	HistoryRecorder               history.Recorder
	DetailedListDir               string
	CommandParameters             history.CommandParameters
	UserTempDiscoveryOptions      UserTempDiscoveryOptions
	DiscoverUserTempOpportunities func(context.Context) UserTempDiscoveryResult
	DiscoverReviewSuggestions     func(context.Context) []ReviewSuggestion
}

type Rule struct {
	ID                    string
	Description           string
	DefaultEnabled        bool
	Roots                 []string
	CandidatePaths        []string
	CandidateNamePrefixes []string
}

type Result struct {
	Status                           string                            `json:"status"`
	Mode                             string                            `json:"mode"`
	DefaultRuleCatalog               []RuleSummary                     `json:"default_rule_catalog"`
	ProtectionRules                  []ProtectionRule                  `json:"protection_rules"`
	Candidates                       []CandidatePreview                `json:"candidates"`
	Deleted                          []DeletedItem                     `json:"deleted"`
	Skipped                          []SkippedItem                     `json:"skipped"`
	Errors                           []StructuredIssue                 `json:"errors"`
	Opportunities                    []UserTempOpportunity             `json:"opportunities"`
	IncompleteOpportunityInspections []IncompleteOpportunityInspection `json:"incomplete_opportunity_inspections"`
	ReviewSuggestions                []ReviewSuggestion                `json:"review_suggestions"`
	Totals                           Totals                            `json:"totals"`
	DetailedListPath                 string                            `json:"-"`
	ElapsedMS                        int64                             `json:"elapsed_ms"`
}

type ProtectionRule struct {
	Path string `json:"path"`
}

type RuleSummary struct {
	ID             string `json:"id"`
	Description    string `json:"description"`
	DefaultEnabled bool   `json:"default_enabled"`
}

type CandidatePreview struct {
	Path          string `json:"path"`
	Bytes         int64  `json:"bytes"`
	Rule          string `json:"rule"`
	PlannedAction string `json:"planned_action"`
}

type SkippedItem struct {
	Path   string          `json:"path"`
	Bytes  int64           `json:"bytes"`
	Rule   string          `json:"rule"`
	Reason StructuredIssue `json:"reason"`
}

type DeletedItem struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Rule  string `json:"rule"`
}

type ReviewSuggestion struct {
	Tool      string `json:"tool"`
	Label     string `json:"label"`
	Command   string `json:"command"`
	CachePath string `json:"cache_path"`
}

type StructuredIssue struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
	Path        string `json:"path,omitempty"`
	Rule        string `json:"rule,omitempty"`
}

type Totals struct {
	CandidateCount           int   `json:"candidate_count"`
	DeletedCount             int   `json:"deleted_count"`
	SkippedCount             int   `json:"skipped_count"`
	OpportunityCount         int   `json:"opportunity_count"`
	CandidateBytes           int64 `json:"candidate_bytes"`
	OpportunityObservedBytes int64 `json:"opportunity_observed_bytes"`
	AffectedBytes            int64 `json:"affected_bytes"`
}

func DryRun(ctx context.Context, opts Options) Result {
	return dryRun(ctx, opts)
}

func dryRun(ctx context.Context, opts Options) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	result := scanDefaultCandidates(ctx, opts, start)
	discover := opts.DiscoverUserTempOpportunities
	if discover == nil {
		discover = func(ctx context.Context) UserTempDiscoveryResult {
			return DiscoverUserTempOpportunities(ctx, opts.UserTempDiscoveryOptions)
		}
	}
	discovery := discover(ctx)
	result.Opportunities = append(result.Opportunities, discovery.Opportunities...)
	result.IncompleteOpportunityInspections = append(result.IncompleteOpportunityInspections, discovery.Incomplete...)
	for _, incomplete := range discovery.Incomplete {
		result.Errors = append(result.Errors, incomplete.Reason)
	}
	discoverSuggestions := opts.DiscoverReviewSuggestions
	if discoverSuggestions == nil {
		discoverSuggestions = DiscoverReviewSuggestions
	}
	result.ReviewSuggestions = append(result.ReviewSuggestions, discoverSuggestions(ctx)...)

	result.ElapsedMS = time.Since(start).Milliseconds()
	result.Totals = totals(result)
	if opts.DetailedListDir != "" {
		path, err := writeDetailedCandidateList(opts.DetailedListDir, result, time.Now())
		if err != nil {
			result.Errors = append(result.Errors, issue("detailed_list_write_failed", err.Error(), true, opts.DetailedListDir, ""))
			result.Totals = totals(result)
		} else {
			result.DetailedListPath = path
		}
	}
	recordHistorySession(ctx, opts, result, start, time.Now())
	return result
}

func scanDefaultCandidates(ctx context.Context, opts Options, start time.Time) Result {
	rules := opts.Rules
	if len(rules) == 0 {
		rules = DefaultRuleCatalog()
	}

	result := Result{
		Status:                           "preview",
		Mode:                             "dry_run",
		DefaultRuleCatalog:               defaultRuleSummaries(rules),
		ProtectionRules:                  protectionRules(opts.Validator),
		Candidates:                       []CandidatePreview{},
		Deleted:                          []DeletedItem{},
		Skipped:                          []SkippedItem{},
		Errors:                           []StructuredIssue{},
		Opportunities:                    []UserTempOpportunity{},
		IncompleteOpportunityInspections: []IncompleteOpportunityInspection{},
		ReviewSuggestions:                []ReviewSuggestion{},
	}

	for _, rule := range rules {
		if !rule.DefaultEnabled {
			continue
		}
		for _, path := range rule.CandidatePaths {
			previewCandidate(ctx, opts.Validator, path, rule.ID, &result)
		}
		for _, root := range rule.Roots {
			select {
			case <-ctx.Done():
				result.Errors = append(result.Errors, issue("context_canceled", ctx.Err().Error(), true, root, rule.ID))
				result.ElapsedMS = time.Since(start).Milliseconds()
				result.Totals = totals(result)
				return result
			default:
			}

			entries, err := os.ReadDir(root)
			if err != nil {
				result.Errors = append(result.Errors, issue(classifyError(err), err.Error(), true, root, rule.ID))
				continue
			}
			for _, entry := range entries {
				if !matchesRuleName(entry.Name(), rule.CandidateNamePrefixes) {
					continue
				}
				path := filepath.Join(root, entry.Name())
				previewCandidate(ctx, opts.Validator, path, rule.ID, &result)
			}
		}
	}

	result.ElapsedMS = time.Since(start).Milliseconds()
	result.Totals = totals(result)
	return result
}

func Execute(ctx context.Context, opts Options) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	result := scanDefaultCandidates(ctx, opts, start)
	result.Status = "ok"
	result.Mode = "execute"
	result.Deleted = []DeletedItem{}

	adapter := opts.RecycleBinAdapter
	if adapter == nil {
		adapter = delete.WindowsRecycleBinAdapter{}
	}

	candidates := make([]delete.Candidate, 0, len(result.Candidates))
	rulesByPath := make(map[string]string, len(result.Candidates))
	for _, candidate := range result.Candidates {
		candidates = append(candidates, delete.Candidate{
			Path:  candidate.Path,
			Bytes: candidate.Bytes,
		})
		rulesByPath[candidate.Path] = candidate.Rule
	}

	deleteResult := delete.ExecuteWithValidator(ctx, candidates, adapter, opts.Validator)
	for _, item := range deleteResult.Deleted {
		result.Deleted = append(result.Deleted, DeletedItem{
			Path:  item.Path,
			Bytes: item.Bytes,
			Rule:  rulesByPath[item.Path],
		})
	}
	for _, item := range deleteResult.Skipped {
		ruleID := rulesByPath[item.Path]
		result.Skipped = append(result.Skipped, SkippedItem{
			Path:   item.Path,
			Bytes:  item.Bytes,
			Rule:   ruleID,
			Reason: issue(item.Reason.Code, item.Reason.Message, true, item.Path, ruleID),
		})
	}

	result.ElapsedMS = time.Since(start).Milliseconds()
	result.Totals = totals(result)
	recordHistorySession(ctx, opts, result, start, time.Now())
	return result
}

func recordHistorySession(ctx context.Context, opts Options, result Result, startedAt, endedAt time.Time) {
	if opts.HistoryRecorder == nil {
		return
	}
	result = withoutOpportunityReviewData(result)
	session := history.SessionRecord{
		ID:        newHistorySessionID(result.Mode, endedAt),
		Command:   opts.CommandParameters,
		StartedAt: startedAt.UTC(),
		EndedAt:   endedAt.UTC(),
		Mode:      result.Mode,
		Aggregate: history.AggregateOutcomes{
			CandidateCount:           result.Totals.CandidateCount,
			DeletedCount:             result.Totals.DeletedCount,
			SkippedCount:             result.Totals.SkippedCount,
			ErrorCount:               len(result.Errors),
			OpportunityCount:         result.Totals.OpportunityCount,
			CandidateBytes:           result.Totals.CandidateBytes,
			OpportunityObservedBytes: result.Totals.OpportunityObservedBytes,
			AffectedBytes:            result.Totals.AffectedBytes,
		},
	}
	_ = opts.HistoryRecorder.Record(ctx, session, historyItems(session.ID, result))
}

func withoutOpportunityReviewData(result Result) Result {
	if len(result.IncompleteOpportunityInspections) == 0 {
		result.Opportunities = nil
		return result
	}
	incompleteIssues := make(map[string]struct{}, len(result.IncompleteOpportunityInspections))
	for _, incomplete := range result.IncompleteOpportunityInspections {
		incompleteIssues[structuredIssueKey(incomplete.Reason)] = struct{}{}
	}
	errorsForHistory := make([]StructuredIssue, 0, len(result.Errors))
	for _, issue := range result.Errors {
		if _, reviewOnly := incompleteIssues[structuredIssueKey(issue)]; reviewOnly {
			continue
		}
		errorsForHistory = append(errorsForHistory, issue)
	}
	result.Opportunities = nil
	result.IncompleteOpportunityInspections = nil
	result.Errors = errorsForHistory
	return result
}

func structuredIssueKey(issue StructuredIssue) string {
	return issue.Path + "\x00" + issue.Code + "\x00" + issue.Message
}

func newHistorySessionID(mode string, at time.Time) string {
	return fmt.Sprintf("clean-%s-%s", mode, at.UTC().Format("20060102T150405.000000000Z"))
}

func historyItems(sessionID string, result Result) []history.ItemRecord {
	items := make([]history.ItemRecord, 0, len(result.Candidates)+len(result.Deleted)+len(result.Skipped)+len(result.Errors))
	if result.Mode == "dry_run" {
		for _, candidate := range result.Candidates {
			bytes := candidate.Bytes
			items = append(items, history.ItemRecord{
				SessionID:     sessionID,
				Path:          candidate.Path,
				Rule:          candidate.Rule,
				PlannedAction: candidate.PlannedAction,
				Bytes:         &bytes,
				Result:        "candidate",
			})
		}
	}
	for _, deleted := range result.Deleted {
		bytes := deleted.Bytes
		items = append(items, history.ItemRecord{
			SessionID: sessionID,
			Path:      deleted.Path,
			Rule:      deleted.Rule,
			Action:    plannedRecycleBinAction,
			Bytes:     &bytes,
			Result:    "deleted",
		})
	}
	for _, skipped := range result.Skipped {
		item := history.ItemRecord{
			SessionID:     sessionID,
			Path:          skipped.Path,
			Rule:          skipped.Rule,
			PlannedAction: plannedRecycleBinAction,
			Result:        "skipped",
			SkippedReason: historyIssue(skipped.Reason),
		}
		if skipped.Bytes > 0 {
			bytes := skipped.Bytes
			item.Bytes = &bytes
		}
		items = append(items, item)
	}
	for _, err := range result.Errors {
		items = append(items, history.ItemRecord{
			SessionID: sessionID,
			Path:      err.Path,
			Rule:      err.Rule,
			Result:    "error",
			Error:     historyIssue(err),
		})
	}
	return items
}

func historyIssue(issue StructuredIssue) *history.Issue {
	return &history.Issue{
		Code:        issue.Code,
		Message:     issue.Message,
		Recoverable: issue.Recoverable,
	}
}

func writeDetailedCandidateList(dir string, result Result, at time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("clean-dry-run-%s-detailed-candidates.txt", at.UTC().Format("20060102T150405.000000000Z")))
	var builder strings.Builder
	builder.WriteString("Foal clean detailed candidate list\n")
	builder.WriteString("Companion file only. This is not an execution manifest; foal clean --execute performs a fresh scan and path-safety validation before using the Recycle Bin.\n\n")

	builder.WriteString("Default candidates\n")
	if len(result.Candidates) == 0 {
		builder.WriteString("  No default candidates found.\n")
	} else {
		for _, candidate := range result.Candidates {
			builder.WriteString(fmt.Sprintf("  path: %s\n", candidate.Path))
			builder.WriteString(fmt.Sprintf("    rule: %s\n", candidate.Rule))
			builder.WriteString(fmt.Sprintf("    bytes: %d\n", candidate.Bytes))
			builder.WriteString(fmt.Sprintf("    planned action: %s (%s)\n", plannedActionLabel(candidate.PlannedAction), candidate.PlannedAction))
		}
	}

	builder.WriteString("\nSkipped-by-default opportunities\n")
	builder.WriteString("  This section is review-only and is not an execution manifest. Opportunity bytes are not counted as Potential space.\n")
	if len(result.Opportunities) == 0 {
		builder.WriteString("  No skipped-by-default opportunities found.\n")
	} else {
		for _, opportunity := range result.Opportunities {
			builder.WriteString(fmt.Sprintf("  path: %s\n", opportunity.Path))
			builder.WriteString(fmt.Sprintf("    bytes: %d\n", opportunity.Bytes))
			builder.WriteString(fmt.Sprintf("    latest modified: %s\n", opportunity.LatestModifiedAt.UTC().Format(time.RFC3339)))
			builder.WriteString(fmt.Sprintf("    idle days: %d\n", opportunity.IdleDays))
			builder.WriteString(fmt.Sprintf("    status: %s\n", opportunity.Status))
			builder.WriteString(fmt.Sprintf("    reason: %s\n", opportunity.Reason))
			builder.WriteString("    not counted as Potential space: true\n")
		}
	}

	builder.WriteString("\nSkipped items\n")
	if len(result.Skipped) == 0 {
		builder.WriteString("  No skipped cleanup paths reported.\n")
	} else {
		for _, skipped := range result.Skipped {
			builder.WriteString(fmt.Sprintf("  path: %s\n", skipped.Path))
			builder.WriteString(fmt.Sprintf("    rule: %s\n", skipped.Rule))
			if skipped.Bytes > 0 {
				builder.WriteString(fmt.Sprintf("    bytes: %d\n", skipped.Bytes))
			}
			builder.WriteString(fmt.Sprintf("    reason: %s\n", skipped.Reason.Code))
			if skipped.Reason.Message != "" {
				builder.WriteString(fmt.Sprintf("    message: %s\n", skipped.Reason.Message))
			}
			builder.WriteString(fmt.Sprintf("    recoverable: %t\n", skipped.Reason.Recoverable))
		}
	}

	builder.WriteString("\nRecoverable errors\n")
	if len(result.Errors) == 0 {
		builder.WriteString("  No recoverable inspection errors reported.\n")
	} else {
		for _, err := range result.Errors {
			builder.WriteString(fmt.Sprintf("  path: %s\n", err.Path))
			builder.WriteString(fmt.Sprintf("    rule: %s\n", err.Rule))
			builder.WriteString(fmt.Sprintf("    error: %s\n", err.Code))
			if err.Message != "" {
				builder.WriteString(fmt.Sprintf("    message: %s\n", err.Message))
			}
			builder.WriteString(fmt.Sprintf("    recoverable: %t\n", err.Recoverable))
		}
	}

	return path, os.WriteFile(path, []byte(builder.String()), 0600)
}

func plannedActionLabel(action string) string {
	if action == plannedRecycleBinAction {
		return "Recycle Bin"
	}
	return action
}

func DefaultRuleCatalog() []Rule {
	return []Rule{
		{
			ID:             "foal_owned_temp_sandboxes",
			Description:    "Foal-owned temporary sandbox entries in the current user's Windows temp directory",
			DefaultEnabled: true,
			Roots:          []string{os.TempDir()},
			CandidateNamePrefixes: []string{
				"foal-",
				"Foal-",
			},
		},
	}
}

func previewCandidate(ctx context.Context, validator pathsafe.Validator, path, ruleID string, result *Result) {
	select {
	case <-ctx.Done():
		result.Skipped = append(result.Skipped, SkippedItem{
			Path:   path,
			Rule:   ruleID,
			Reason: issue("context_canceled", ctx.Err().Error(), true, path, ruleID),
		})
		return
	default:
	}

	if reason, ok := validator.ValidateDeletePath(path); !ok {
		result.Skipped = append(result.Skipped, SkippedItem{
			Path:   path,
			Rule:   ruleID,
			Reason: fromPathsafeReason(reason, path, ruleID),
		})
		return
	}

	bytes, err := measureBytes(path)
	if err != nil {
		result.Skipped = append(result.Skipped, SkippedItem{
			Path:   path,
			Rule:   ruleID,
			Reason: issue(classifyError(err), err.Error(), true, path, ruleID),
		})
		return
	}

	result.Candidates = append(result.Candidates, CandidatePreview{
		Path:          path,
		Bytes:         bytes,
		Rule:          ruleID,
		PlannedAction: plannedRecycleBinAction,
	})
}

func protectionRules(validator pathsafe.Validator) []ProtectionRule {
	paths := validator.UserProtectionPaths()
	rules := make([]ProtectionRule, 0, len(paths))
	for _, path := range paths {
		rules = append(rules, ProtectionRule{Path: path})
	}
	return rules
}

func measureBytes(path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func matchesRuleName(name string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	for _, prefix := range prefixes {
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func defaultRuleSummaries(rules []Rule) []RuleSummary {
	summaries := make([]RuleSummary, 0, len(rules))
	for _, rule := range rules {
		summaries = append(summaries, RuleSummary{
			ID:             rule.ID,
			Description:    rule.Description,
			DefaultEnabled: rule.DefaultEnabled,
		})
	}
	return summaries
}

func fromPathsafeReason(reason pathsafe.Reason, path, ruleID string) StructuredIssue {
	return issue(reason.Code, reason.Message, true, path, ruleID)
}

func issue(code, message string, recoverable bool, path, ruleID string) StructuredIssue {
	return StructuredIssue{
		Code:        code,
		Message:     message,
		Recoverable: recoverable,
		Path:        path,
		Rule:        ruleID,
	}
}

func classifyError(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "not_found"
	case errors.Is(err, fs.ErrPermission):
		return "permission_denied"
	default:
		return "inspection_failed"
	}
}

func totals(result Result) Totals {
	var bytes int64
	for _, candidate := range result.Candidates {
		bytes += candidate.Bytes
	}
	var affectedBytes int64
	for _, deleted := range result.Deleted {
		affectedBytes += deleted.Bytes
	}
	var opportunityBytes int64
	for _, opportunity := range result.Opportunities {
		opportunityBytes += opportunity.Bytes
	}
	return Totals{
		CandidateCount:           len(result.Candidates),
		DeletedCount:             len(result.Deleted),
		SkippedCount:             len(result.Skipped),
		OpportunityCount:         len(result.Opportunities),
		CandidateBytes:           bytes,
		OpportunityObservedBytes: opportunityBytes,
		AffectedBytes:            affectedBytes,
	}
}
