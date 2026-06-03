package clean

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

const plannedRecycleBinAction = "move_to_recycle_bin"

type Options struct {
	Rules []Rule
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

type StructuredIssue struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
	Path        string `json:"path,omitempty"`
	Rule        string `json:"rule,omitempty"`
}

type Totals struct {
	CandidateCount int   `json:"candidate_count"`
	SkippedCount   int   `json:"skipped_count"`
	CandidateBytes int64 `json:"candidate_bytes"`
}

func DryRun(ctx context.Context, opts Options) Result {
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
	return result
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
	return Totals{
		CandidateCount: len(result.Candidates),
		SkippedCount:   len(result.Skipped),
		CandidateBytes: bytes,
	}
}
