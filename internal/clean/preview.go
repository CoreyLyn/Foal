package clean

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/delete"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/history"
)

const plannedRecycleBinAction = "move_to_recycle_bin"

type Options struct {
	Rules             []Rule
	RecycleBinAdapter delete.Adapter
	HistoryRecorder   history.Recorder
	CommandParameters history.CommandParameters
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
	Status             string             `json:"status"`
	Mode               string             `json:"mode"`
	DefaultRuleCatalog []RuleSummary      `json:"default_rule_catalog"`
	Candidates         []CandidatePreview `json:"candidates"`
	Deleted            []DeletedItem      `json:"deleted"`
	Skipped            []SkippedItem      `json:"skipped"`
	Errors             []StructuredIssue  `json:"errors"`
	Totals             Totals             `json:"totals"`
	ElapsedMS          int64              `json:"elapsed_ms"`
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

type StructuredIssue struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
	Path        string `json:"path,omitempty"`
	Rule        string `json:"rule,omitempty"`
}

type Totals struct {
	CandidateCount int   `json:"candidate_count"`
	DeletedCount   int   `json:"deleted_count"`
	SkippedCount   int   `json:"skipped_count"`
	CandidateBytes int64 `json:"candidate_bytes"`
	AffectedBytes  int64 `json:"affected_bytes"`
}

func DryRun(ctx context.Context, opts Options) Result {
	return dryRun(ctx, opts, true)
}

func dryRun(ctx context.Context, opts Options, recordHistory bool) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	rules := opts.Rules
	if len(rules) == 0 {
		rules = DefaultRuleCatalog()
	}

	result := Result{
		Status:             "preview",
		Mode:               "dry_run",
		DefaultRuleCatalog: defaultRuleSummaries(rules),
		Candidates:         []CandidatePreview{},
		Deleted:            []DeletedItem{},
		Skipped:            []SkippedItem{},
		Errors:             []StructuredIssue{},
	}

	for _, rule := range rules {
		if !rule.DefaultEnabled {
			continue
		}
		for _, path := range rule.CandidatePaths {
			previewCandidate(ctx, path, rule.ID, &result)
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
				previewCandidate(ctx, path, rule.ID, &result)
			}
		}
	}

	result.ElapsedMS = time.Since(start).Milliseconds()
	result.Totals = totals(result)
	if recordHistory {
		recordHistorySession(ctx, opts, result, start, time.Now())
	}
	return result
}

func Execute(ctx context.Context, opts Options) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	result := dryRun(ctx, opts, false)
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

	deleteResult := delete.Execute(ctx, candidates, adapter)
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
	session := history.SessionRecord{
		ID:        newHistorySessionID(result.Mode, endedAt),
		Command:   opts.CommandParameters,
		StartedAt: startedAt.UTC(),
		EndedAt:   endedAt.UTC(),
		Mode:      result.Mode,
		Aggregate: history.AggregateOutcomes{
			CandidateCount: result.Totals.CandidateCount,
			DeletedCount:   result.Totals.DeletedCount,
			SkippedCount:   result.Totals.SkippedCount,
			ErrorCount:     len(result.Errors),
			CandidateBytes: result.Totals.CandidateBytes,
			AffectedBytes:  result.Totals.AffectedBytes,
		},
	}
	_ = opts.HistoryRecorder.Record(ctx, session, historyItems(session.ID, result))
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

func previewCandidate(ctx context.Context, path, ruleID string, result *Result) {
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

	if reason, ok := pathsafe.ValidateDeletePath(path); !ok {
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
	return Totals{
		CandidateCount: len(result.Candidates),
		DeletedCount:   len(result.Deleted),
		SkippedCount:   len(result.Skipped),
		CandidateBytes: bytes,
		AffectedBytes:  affectedBytes,
	}
}
