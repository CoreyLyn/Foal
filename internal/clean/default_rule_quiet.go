package clean

import (
	"context"
	"path/filepath"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// foalOwnedTempQuietPeriod is the minimum age of an entry's deep latest observed
// modification before the default foal_owned_temp_sandboxes rule may select it.
// The foal-/Foal- prefix expresses intended ownership, not inactivity: a sandbox
// created minutes ago may still be in use, so plain `foal clean --execute` must
// leave it alone until it has been quiet for a full day.
const foalOwnedTempQuietPeriod = 24 * time.Hour

// PreviewReasonFoalOwnedTempRecent is the stable recoverable skip reason for a
// quiet-period-gated default rule entry that was modified within its window, has
// a future timestamp, or changed again before mutation.
const PreviewReasonFoalOwnedTempRecent = "foal_owned_temp_recent"

func defaultRuleNow(opts Options) time.Time {
	if opts.DefaultRuleNow != nil {
		return opts.DefaultRuleNow()
	}
	return time.Now()
}

// ruleQuietPeriodIssue reports whether path has been quiet for at least quiet.
// It walks the whole entry (the root's own timestamp included) and fails closed:
// inspection limits, reparse points, unreadable descendants, recent writes, and
// future timestamps all return a recoverable skip issue. A non-positive quiet
// period is always satisfied.
func ruleQuietPeriodIssue(ctx context.Context, path, ruleID string, quiet time.Duration, now time.Time) (StructuredIssue, bool) {
	if quiet <= 0 {
		return StructuredIssue{}, true
	}
	inspection, err := inspectOpportunity(ctx, path, userTempDescendantLimit, filepath.WalkDir)
	if err != nil {
		return issue(classifyOpportunityInspectionError(err), err.Error(), true, path, ruleID), false
	}
	latest := inspection.latestModifiedAt
	if latest.After(now) || now.Sub(latest) < quiet {
		return issue(PreviewReasonFoalOwnedTempRecent,
			"entry was modified within the quiet period or has a future timestamp; Foal-owned temp sandbox was skipped",
			true, path, ruleID), false
	}
	return StructuredIssue{}, true
}

// ruleQuietPeriods maps rule IDs to their positive quiet periods so execution
// can repeat the gate immediately before mutation.
func ruleQuietPeriods(opts Options) map[string]time.Duration {
	rules := opts.Rules
	if len(rules) == 0 {
		rules = DefaultRuleCatalog()
	}
	periods := map[string]time.Duration{}
	for _, rule := range rules {
		if rule.MinimumQuietPeriod > 0 {
			periods[rule.ID] = rule.MinimumQuietPeriod
		}
	}
	return periods
}

// quietPeriodPreMutation repeats a rule's quiet-period gate immediately before
// mutation. A candidate written to between resolution and mutation is skipped
// with the same stable reason; it never falls back to another action.
func quietPeriodPreMutation(opts Options, candidate actionExecutionCandidate) (pathsafe.Reason, bool) {
	if candidate.quietPeriod <= 0 {
		return pathsafe.Reason{}, true
	}
	problem, ok := ruleQuietPeriodIssue(context.Background(), candidate.candidate.Path, candidate.rule, candidate.quietPeriod, defaultRuleNow(opts))
	if ok {
		return pathsafe.Reason{}, true
	}
	return pathsafe.Reason{Code: problem.Code, Message: problem.Message}, false
}
