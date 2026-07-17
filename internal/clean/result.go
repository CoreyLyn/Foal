package clean

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

func protectionLoadFailure(mode string, opts Options, start time.Time) Result {
	loadError := *opts.ProtectionLoadError
	rules := opts.Rules
	if len(rules) == 0 {
		rules = DefaultRuleCatalog()
	}
	return Result{
		Status:                           "error",
		Mode:                             mode,
		DefaultRuleCatalog:               defaultRuleSummaries(rules),
		ProtectionRules:                  []ProtectionRule{},
		ProtectionDiagnostics:            append([]ProtectionDiagnostic(nil), opts.ProtectionDiagnostics...),
		Candidates:                       []CandidatePreview{},
		Deleted:                          []DeletedItem{},
		Failed:                           []FailedItem{},
		Skipped:                          []SkippedItem{},
		Errors:                           []StructuredIssue{loadError},
		Opportunities:                    []Opportunity{},
		IncompleteOpportunityInspections: []IncompleteOpportunityInspection{},
		ReviewSuggestions:                []ReviewSuggestion{},
		RunningApplications:              []RunningApplicationState{},
		Totals:                           Totals{},
		ElapsedMS:                        time.Since(start).Milliseconds(),
	}
}

func resolveApplicationCacheDiscovery(ctx context.Context, opts Options, policyID string) applicationCacheDiscoveryResult {
	if opts.DiscoverApplicationCaches != nil {
		return opts.DiscoverApplicationCaches(ctx, policyID, opts.ApplicationCacheDiscoveryOptions, opts.Validator)
	}
	return discoverApplicationCaches(ctx, policyID, opts.ApplicationCacheDiscoveryOptions, opts.Validator)
}

func browserCacheLocalAppDataDir(opts BrowserCacheDiscoveryOptions) string {
	if opts.LocalAppDataDir != "" {
		return opts.LocalAppDataDir
	}
	return os.Getenv("LOCALAPPDATA")
}

func browserCacheRoamingAppDataDir(opts BrowserCacheDiscoveryOptions) string {
	if opts.RoamingAppDataDir != "" {
		return opts.RoamingAppDataDir
	}
	return os.Getenv("APPDATA")
}

func runningApplicationStateFor(states []RunningApplicationState, application string) (RunningApplicationState, bool) {
	for _, state := range states {
		if state.Application == application {
			return state, true
		}
	}
	return RunningApplicationState{}, false
}

func replaceRunningApplicationState(result *Result, replacement RunningApplicationState) {
	result.RunningApplications = mergeRunningApplicationStates(result.RunningApplications, replacement)
}

func browserOpportunityProtected(validator pathsafe.Validator, opportunity Opportunity) bool {
	if validator.IsUserProtected(opportunity.Path) {
		return true
	}
	if opportunity.BrowserCache == nil {
		return false
	}
	for _, profile := range opportunity.BrowserCache.Profiles {
		if validator.IsUserProtected(profile.Path) {
			return true
		}
		for _, cache := range profile.Caches {
			if validator.IsUserProtected(cache.Path) {
				return true
			}
		}
	}
	return false
}

func browserOpportunityPathProtected(validator pathsafe.Validator, incomplete *IncompleteOpportunityInspection) bool {
	return incomplete != nil && validator.IsUserProtected(incomplete.Path)
}

func suppressProtectionRules(result *Result, suppressedPaths []string) {
	if len(suppressedPaths) == 0 || len(result.ProtectionRules) == 0 {
		return
	}
	suppressed := make(map[string]struct{}, len(suppressedPaths))
	for _, path := range suppressedPaths {
		suppressed[strings.ToLower(filepath.Clean(path))] = struct{}{}
	}
	filtered := result.ProtectionRules[:0]
	for _, rule := range result.ProtectionRules {
		if _, ok := suppressed[strings.ToLower(filepath.Clean(rule.Path))]; ok {
			continue
		}
		filtered = append(filtered, rule)
	}
	result.ProtectionRules = filtered
}

func runningApplicationUnknownIssue(state RunningApplicationState) StructuredIssue {
	message := state.Message
	if message == "" {
		message = applicationDisplayName(state.Application) + " process state could not be determined; cache review was skipped."
	}
	rule := "cache_review"
	if isBrowserApplication(state.Application) {
		rule = "browser_review"
	}
	return issue(runningApplicationDetectionIssueCode, message, true, "", rule)
}

// plannedActionForCategory returns the catalog-owned planned action for a
// canonical category. Injected test rules outside the catalog stay Recycle Bin
// only; permanent deletion is never inferred for non-catalog identifiers.
func plannedActionForCategory(identifier string) string {
	return resolvePlannedAction(identifier, nil)
}

// plannedActionForOpts resolves the planned action for identifier using any
// per-run CategoryPlannedActions override on opts before the catalog.
func plannedActionForOpts(opts Options, identifier string) string {
	return resolvePlannedAction(identifier, opts.CategoryPlannedActions)
}

// resolvePlannedAction returns the planned action for identifier, honoring an
// optional per-run test override map before the catalog. Production leaves
// overrides nil so the catalog remains the sole source of truth.
func resolvePlannedAction(identifier string, overrides map[string]DeletionAction) string {
	if action, ok := overrides[identifier]; ok && validDeletionAction(action) {
		return string(action)
	}
	if summary, ok := canonicalCleanupCategoryCatalog.Summary(identifier); ok {
		if summary.PlannedAction == "" {
			return ""
		}
		return string(summary.PlannedAction)
	}
	return plannedRecycleBinAction
}

func DefaultRuleCatalog() []Rule {
	return []Rule{
		{
			ID:             DefaultCategoryFoalOwnedTempSandboxes,
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

func previewCandidate(ctx context.Context, opts Options, path, ruleID string, result *Result) {
	planned := resolvePlannedAction(ruleID, opts.CategoryPlannedActions)
	select {
	case <-ctx.Done():
		result.Skipped = append(result.Skipped, SkippedItem{
			Path:          path,
			Rule:          ruleID,
			PlannedAction: planned,
			Reason:        issue("context_canceled", ctx.Err().Error(), true, path, ruleID),
		})
		return
	default:
	}

	if reason, ok := opts.Validator.ValidateDeletePath(path); !ok {
		result.Skipped = append(result.Skipped, SkippedItem{
			Path:          path,
			Rule:          ruleID,
			PlannedAction: planned,
			Reason:        fromPathsafeReason(reason, path, ruleID),
		})
		return
	}

	bytes, err := measureBytes(ctx, path)
	if err != nil {
		result.Skipped = append(result.Skipped, SkippedItem{
			Path:          path,
			Rule:          ruleID,
			PlannedAction: planned,
			Reason:        issue(classifyError(err), err.Error(), true, path, ruleID),
		})
		return
	}

	result.Candidates = append(result.Candidates, CandidatePreview{
		Path:          path,
		Bytes:         bytes,
		Rule:          ruleID,
		PlannedAction: planned,
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

func measureBytes(ctx context.Context, path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
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
	if err != nil {
		return 0, err
	}
	return total, nil
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

// NormalizedOptInSet returns the set of opt-in categories enabled, resolving
// group tokens: "all" to every implemented opt-in category, "dev-caches" to
// developer-cache plus Application cache categories, and "cli-agents" to
// independently registered product-scoped CLI-agent categories. Group tokens
// own no resolver, candidates, or deletion action.
// Returns the set, a list of invalid names (if any), and the list of valid
// names for error reporting.
func NormalizedOptInSet(optIn []string) (enabled map[string]bool, invalid []string, valid []string) {
	selectable := selectableCategoryIDs()
	// dev-caches expands from catalog policy: developer-cache categories plus
	// idle Application cache opportunities under Developer tools.
	devCaches := developerToolsOptInCategoryIDs()
	// cli-agents expands independently registered product-scoped CLI-agent
	// categories in deterministic catalog order (not a mega-category).
	cliAgents := cliAgentCategoryIDs()
	valid = make([]string, 0, len(selectable)+3)
	valid = append(valid, selectable...)
	valid = append(valid, DevCacheCategoryAll, CLIAgentCategoryGroup, "all")

	enabled = make(map[string]bool)
	seen := make(map[string]bool)
	all := false
	allDevCaches := false
	allCLIAgents := false
	for _, name := range optIn {
		name = strings.ToLower(name)
		if seen[name] {
			continue
		}
		seen[name] = true
		if name == "all" {
			all = true
			continue
		}
		if name == DevCacheCategoryAll {
			allDevCaches = true
			continue
		}
		if name == CLIAgentCategoryGroup {
			allCLIAgents = true
			continue
		}
		summary, validName := canonicalCleanupCategoryCatalog.Summary(name)
		validName = validName && strings.TrimSpace(name) == name
		if !validName || summary.Eligibility != CategoryEligibilityOptIn {
			invalid = append(invalid, name)
			continue
		}
		enabled[summary.Identifier] = true
	}
	if all {
		for _, v := range selectable {
			enabled[v] = true
		}
	} else {
		// Group tokens expand independently and compose with exact tokens.
		if allDevCaches {
			for _, v := range devCaches {
				enabled[v] = true
			}
		}
		if allCLIAgents {
			for _, v := range cliAgents {
				enabled[v] = true
			}
		}
	}
	return enabled, invalid, valid
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
	var recycleBinMovedBytes int64
	var permanentlyDeletedBytes int64
	var optInAffectedBytes int64
	var optInDeletedCount int
	for _, deleted := range result.Deleted {
		switch deleted.Action {
		case string(DeletionActionDeletePermanently):
			permanentlyDeletedBytes += deleted.Bytes
		default:
			// Empty action (legacy History / fixtures) and move_to_recycle_bin
			// both count as Recycle Bin work. Production successes always set
			// an explicit action; empty remains readable for older records.
			recycleBinMovedBytes += deleted.Bytes
		}
		if deleted.IsOptIn {
			optInAffectedBytes += deleted.Bytes
			optInDeletedCount++
		}
	}
	affectedBytes := recycleBinMovedBytes + permanentlyDeletedBytes
	var opportunityBytes int64
	for _, opportunity := range result.Opportunities {
		opportunityBytes += opportunity.Bytes
	}
	var optInReclaimableBytes int64
	for _, candidate := range result.OptInCandidates {
		optInReclaimableBytes += candidate.Bytes
	}
	return Totals{
		CandidateCount:           len(result.Candidates),
		DeletedCount:             len(result.Deleted),
		// Failed permanent candidates are not successes; surface them with skips
		// so aggregate counts stay path-complete without claiming permanent bytes.
		SkippedCount:             len(result.Skipped) + len(result.Failed),
		OpportunityCount:         len(result.Opportunities),
		CandidateBytes:           bytes,
		OpportunityObservedBytes: opportunityBytes,
		OptInCandidateCount:      len(result.OptInCandidates),
		OptInReclaimableBytes:    optInReclaimableBytes,
		OptInDeletedCount:        optInDeletedCount,
		OptInAffectedBytes:       optInAffectedBytes,
		RecycleBinMovedBytes:     recycleBinMovedBytes,
		PermanentlyDeletedBytes:  permanentlyDeletedBytes,
		AffectedBytes:            affectedBytes,
	}
}
