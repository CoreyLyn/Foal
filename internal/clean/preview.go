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

// Dev cache categories - these are Review suggestions that become opt-in candidates
const (
	DevCacheCategoryNPM      = "npm-cache"
	DevCacheCategoryGo       = "go-cache"
	DevCacheCategoryPip      = "pip-cache"
	DevCacheCategoryCargo    = "cargo-cache"
	DevCacheCategoryNuGet    = "nuget-cache"
	DevCacheCategoryCorepack = "corepack-cache"
	DevCacheCategoryAll      = "dev-caches"
)

// RecycleBinVolumeConfig holds the Recycle Bin configuration for a volume.
type RecycleBinVolumeConfig struct {
	// NukeOnDelete is true if the Recycle Bin is disabled for this volume
	// (items are permanently deleted immediately).
	NukeOnDelete bool
	// MaxCapacity is the maximum size in bytes that can be stored in the
	// Recycle Bin for this volume.
	MaxCapacity int64
}

// RecycleBinCapacityProbe returns the Recycle Bin configuration for the
// volume containing the given path.
type RecycleBinCapacityProbe func(path string) (RecycleBinVolumeConfig, error)

// DevCachePathResolver resolves the path for a developer tool cache.
// The category is one of the DevCacheCategory* constants.
// Returns empty string if the path cannot be resolved from env vars/defaults.
type DevCachePathResolver func(category string) string

type Options struct {
	Rules                         []Rule
	Validator                     pathsafe.Validator
	ProtectionDiagnostics         []ProtectionDiagnostic
	ProtectionLoadError           *StructuredIssue
	RecycleBinAdapter             delete.Adapter
	HistoryRecorder               history.Recorder
	DetailedListDir               string
	CommandParameters             history.CommandParameters
	UserTempDiscoveryOptions      UserTempDiscoveryOptions
	DiscoverUserTempOpportunities func(context.Context) UserTempDiscoveryResult
	OpportunityDiscoveryOptions   OpportunityDiscoveryOptions
	DiscoverOpportunities         func(context.Context) OpportunityDiscoveryResult
	BrowserCacheDiscoveryOptions  BrowserCacheDiscoveryOptions
	DiscoverReviewSuggestions     func(context.Context) []ReviewSuggestion
	DetectRunningApplications     func(context.Context) []RunningApplicationState
	OptIn                         []string
	RecycleBinCapacityProbe       RecycleBinCapacityProbe
	DevCachePathResolver          DevCachePathResolver
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
	ProtectionDiagnostics            []ProtectionDiagnostic            `json:"protection_diagnostics"`
	Candidates                       []CandidatePreview                `json:"candidates"`
	Deleted                          []DeletedItem                     `json:"deleted"`
	Skipped                          []SkippedItem                     `json:"skipped"`
	Errors                           []StructuredIssue                 `json:"errors"`
	Opportunities                    []Opportunity                     `json:"opportunities"`
	OptInCandidates                  []OptInCandidate                  `json:"opt_in_candidates"`
	IncompleteOpportunityInspections []IncompleteOpportunityInspection `json:"incomplete_opportunity_inspections"`
	ReviewSuggestions                []ReviewSuggestion                `json:"review_suggestions"`
	RunningApplications              []RunningApplicationState         `json:"running_applications"`
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
	Path    string `json:"path"`
	Bytes   int64  `json:"bytes"`
	Rule    string `json:"rule"`
	IsOptIn bool   `json:"is_opt_in,omitempty"`
}

type OptInCandidate struct {
	Path           string `json:"path"`
	Bytes          int64  `json:"bytes"`
	Category       string `json:"category"`
	IsUserTemp     bool   `json:"is_user_temp,omitempty"`
	LatestModified int64  `json:"latest_modified,omitempty"`
	IdleDays       int    `json:"idle_days,omitempty"`
	PlannedAction  string `json:"planned_action"`
}

type ReviewSuggestion struct {
	Tool      string `json:"tool"`
	Label     string `json:"label"`
	Command   string `json:"command"`
	CachePath string `json:"cache_path"`
}

type RunningApplicationStatus string

const (
	ApplicationGoogleChrome                = "google_chrome"
	ApplicationMicrosoftEdge               = "microsoft_edge"
	RunningApplicationStateRunning         = RunningApplicationStatus("running")
	RunningApplicationStateIdle            = RunningApplicationStatus("idle")
	RunningApplicationStateUnknown         = RunningApplicationStatus("unknown")
	runningApplicationDetectionIssueCode   = "running_application_detection_unknown"
	recycleBinDisabledIssueCode            = "recycle_bin_disabled"
	recycleBinCapacityIssueCode            = "recycle_bin_capacity"
	recycleBinCapacityProbeFailedIssueCode = "recycle_bin_capacity_probe_failed"
)

type RunningApplicationState struct {
	Application string                   `json:"application"`
	State       RunningApplicationStatus `json:"state"`
	Message     string                   `json:"message,omitempty"`
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
	OptInCandidateCount      int   `json:"opt_in_candidate_count"`
	OptInReclaimableBytes    int64 `json:"opt_in_reclaimable_bytes"`
	OptInDeletedCount        int   `json:"opt_in_deleted_count"`
	OptInAffectedBytes       int64 `json:"opt_in_affected_bytes"`
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
	if opts.ProtectionLoadError != nil {
		return protectionLoadFailure("dry_run", opts, start)
	}
	result := scanDefaultCandidates(ctx, opts, start)

	// Resolve opt-in set
	optInEnabled, _, _ := NormalizedOptInSet(opts.OptIn)

	// Get dev cache path resolver
	resolveDevCache := opts.DevCachePathResolver
	if resolveDevCache == nil {
		resolveDevCache = ResolveDevCachePath
	}

	// Process opted-in dev caches
	optedInDevCachePaths := make(map[string]bool)
	for category := range optInEnabled {
		if !isDevCacheCategory(category) {
			continue
		}
		path := resolveDevCache(category)
		if path == "" {
			continue
		}
		if opts.Validator.IsUserProtected(path) {
			continue
		}
		// Measure the size
		bytes, err := measureBytes(path)
		if err != nil {
			// Missing or unreadable - just skip, don't add as candidate
			continue
		}
		result.OptInCandidates = append(result.OptInCandidates, OptInCandidate{
			Path:          path,
			Bytes:         bytes,
			Category:      category,
			PlannedAction: plannedRecycleBinAction,
		})
		optedInDevCachePaths[path] = true
	}

	discover := opts.DiscoverOpportunities
	if discover == nil && opts.DiscoverUserTempOpportunities != nil {
		discover = opts.DiscoverUserTempOpportunities
	}
	if discover == nil {
		discover = func(ctx context.Context) OpportunityDiscoveryResult {
			discoveryOptions := opts.OpportunityDiscoveryOptions
			if discoveryOptions.TempDir == "" {
				discoveryOptions.TempDir = opts.UserTempDiscoveryOptions.TempDir
			}
			if discoveryOptions.Now.IsZero() {
				discoveryOptions.Now = opts.UserTempDiscoveryOptions.Now
			}
			return DiscoverOpportunities(ctx, discoveryOptions)
		}
	}
	discovery := discover(ctx)
	for _, opportunity := range discovery.Opportunities {
		opportunity.Category = normalizedOpportunityCategory(opportunity.Category)
		if opts.Validator.IsUserProtected(opportunity.Path) {
			continue
		}
		if optInCategoryEnabled(optInEnabled, opportunity.Category) {
			candidate := OptInCandidate{
				Path:          opportunity.Path,
				Bytes:         opportunity.Bytes,
				Category:      opportunity.Category,
				PlannedAction: plannedRecycleBinAction,
			}
			if opportunity.Category == OpportunityCategoryUserTemp {
				candidate.IsUserTemp = true
				candidate.LatestModified = opportunity.LatestModifiedAt.Unix()
				candidate.IdleDays = opportunity.IdleDays
			}
			result.OptInCandidates = append(result.OptInCandidates, candidate)
		} else {
			result.Opportunities = append(result.Opportunities, opportunity)
		}
	}
	for _, incomplete := range discovery.Incomplete {
		incomplete.Category = normalizedOpportunityCategory(incomplete.Category)
		if opts.Validator.IsUserProtected(incomplete.Path) {
			continue
		}
		result.IncompleteOpportunityInspections = append(result.IncompleteOpportunityInspections, incomplete)
		result.Errors = append(result.Errors, incomplete.Reason)
	}
	discoverSuggestions := opts.DiscoverReviewSuggestions
	if discoverSuggestions == nil {
		discoverSuggestions = DiscoverReviewSuggestions
	}
	for _, suggestion := range discoverSuggestions(ctx) {
		if suggestion.CachePath != "" && opts.Validator.IsUserProtected(suggestion.CachePath) {
			continue
		}
		// Skip review suggestions that are already opted-in as candidates
		if suggestion.CachePath != "" && optedInDevCachePaths[suggestion.CachePath] {
			continue
		}
		// Also skip if the suggestion category is opted-in
		skip := false
		for category := range optInEnabled {
			if isDevCacheCategory(category) && devCacheCategoryMatchesSuggestion(category, suggestion) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		result.ReviewSuggestions = append(result.ReviewSuggestions, suggestion)
	}
	if opts.DetectRunningApplications != nil {
		applyBrowserCacheReview(ctx, opts, &result, optInEnabled)
	}

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

func applyBrowserCacheReview(ctx context.Context, opts Options, result *Result, optInEnabled map[string]bool) {
	preStates := opts.DetectRunningApplications(ctx)
	result.RunningApplications = append(result.RunningApplications, preStates...)
	for _, state := range preStates {
		if state.State == RunningApplicationStateUnknown {
			result.Errors = append(result.Errors, runningApplicationUnknownIssue(state))
		}
	}
	for _, config := range browserCacheConfigs {
		applyOneBrowserCacheReview(ctx, opts, result, preStates, config, optInEnabled)
	}
}

func applyOneBrowserCacheReview(ctx context.Context, opts Options, result *Result, preStates []RunningApplicationState, config browserCacheConfig, optInEnabled map[string]bool) {
	if localAppDataDir := browserCacheLocalAppDataDir(opts.BrowserCacheDiscoveryOptions); localAppDataDir != "" {
		suppressed, protectedRulePaths := browserDiscoverySuppressed(browserUserDataRoot(localAppDataDir, config), opts.Validator)
		if suppressed {
			suppressProtectionRules(result, protectedRulePaths)
			return
		}
	}
	preState, ok := runningApplicationStateFor(preStates, config.application)
	if !ok || preState.State != RunningApplicationStateIdle {
		return
	}

	discovery := discoverBrowserCache(ctx, config, opts.BrowserCacheDiscoveryOptions, opts.Validator)
	if discovery.suppressed {
		suppressProtectionRules(result, discovery.suppressedProtectionPaths)
		return
	}
	if discovery.diagnostic != nil {
		result.Errors = append(result.Errors, *discovery.diagnostic)
		return
	}
	if discovery.incomplete != nil {
		if !browserOpportunityPathProtected(opts.Validator, discovery.incomplete) {
			result.IncompleteOpportunityInspections = append(result.IncompleteOpportunityInspections, *discovery.incomplete)
			result.Errors = append(result.Errors, discovery.incomplete.Reason)
		}
		return
	}

	postStates := opts.DetectRunningApplications(ctx)
	postState, ok := runningApplicationStateFor(postStates, config.application)
	if !ok {
		return
	}
	if postState.State != RunningApplicationStateIdle {
		replaceRunningApplicationState(result, postState)
		if postState.State == RunningApplicationStateUnknown {
			result.Errors = append(result.Errors, runningApplicationUnknownIssue(postState))
		}
		return
	}
	if discovery.opportunity == nil || browserOpportunityProtected(opts.Validator, *discovery.opportunity) {
		return
	}
	if optInCategoryEnabled(optInEnabled, OpportunityCategoryBrowserCache) {
		result.OptInCandidates = append(result.OptInCandidates, OptInCandidate{
			Path:          discovery.opportunity.Path,
			Bytes:         discovery.opportunity.Bytes,
			Category:      OpportunityCategoryBrowserCache,
			PlannedAction: plannedRecycleBinAction,
		})
	} else {
		result.Opportunities = append(result.Opportunities, *discovery.opportunity)
	}
}

func browserCacheLocalAppDataDir(opts BrowserCacheDiscoveryOptions) string {
	if opts.LocalAppDataDir != "" {
		return opts.LocalAppDataDir
	}
	return os.Getenv("LOCALAPPDATA")
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
	for index, state := range result.RunningApplications {
		if state.Application == replacement.Application {
			result.RunningApplications[index] = replacement
			return
		}
	}
	result.RunningApplications = append(result.RunningApplications, replacement)
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
		ProtectionDiagnostics:            append([]ProtectionDiagnostic(nil), opts.ProtectionDiagnostics...),
		Candidates:                       []CandidatePreview{},
		Deleted:                          []DeletedItem{},
		Skipped:                          []SkippedItem{},
		Errors:                           []StructuredIssue{},
		Opportunities:                    []Opportunity{},
		IncompleteOpportunityInspections: []IncompleteOpportunityInspection{},
		ReviewSuggestions:                []ReviewSuggestion{},
		RunningApplications:              []RunningApplicationState{},
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
	if opts.ProtectionLoadError != nil {
		return protectionLoadFailure("execute", opts, start)
	}
	result := scanDefaultCandidates(ctx, opts, start)
	result.Status = "ok"
	result.Mode = "execute"
	result.Deleted = []DeletedItem{}

	adapter := opts.RecycleBinAdapter
	if adapter == nil {
		adapter = delete.WindowsRecycleBinAdapter{}
	}

	// First, handle default candidates
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

	// Now, handle all opted-in categories
	optInEnabled, _, _ := NormalizedOptInSet(opts.OptIn)

	if len(optInEnabled) > 0 {
		// Get the capacity probe (use default if not injected)
		probe := opts.RecycleBinCapacityProbe
		if probe == nil {
			probe = RecycleBinVolumeCapacity
		}

		// Get dev cache path resolver
		resolveDevCache := opts.DevCachePathResolver
		if resolveDevCache == nil {
			resolveDevCache = ResolveDevCachePath
		}

		// Struct to track path with its category and byte size
		type optInPath struct {
			path     string
			bytes    int64
			category string
		}
		var pathsToProcess []optInPath

		// Process dev caches first
		for category := range optInEnabled {
			if !isDevCacheCategory(category) {
				continue
			}
			path := resolveDevCache(category)
			if path == "" {
				continue
			}
			// Fresh measure before deletion
			bytes, err := measureBytes(path)
			if err != nil {
				// Missing or unreadable - skip
				continue
			}
			pathsToProcess = append(pathsToProcess, optInPath{
				path:     path,
				bytes:    bytes,
				category: category,
			})
		}

		// Process non-browser opportunity categories
		if optInCategoryEnabled(optInEnabled, OpportunityCategoryUserTemp) ||
			optInCategoryEnabled(optInEnabled, OpportunityCategoryCrashDumps) ||
			optInCategoryEnabled(optInEnabled, OpportunityCategoryWindowsErrorReporting) ||
			optInCategoryEnabled(optInEnabled, OpportunityCategoryExplorerThumbnailCache) ||
			optInCategoryEnabled(optInEnabled, OpportunityCategoryINetCache) ||
			optInCategoryEnabled(optInEnabled, OpportunityCategoryD3DShaderCache) ||
			optInCategoryEnabled(optInEnabled, OpportunityCategoryNVIDIADXCache) {

			discover := opts.DiscoverOpportunities
			if discover == nil && opts.DiscoverUserTempOpportunities != nil {
				discover = opts.DiscoverUserTempOpportunities
			}
			if discover == nil {
				discover = func(ctx context.Context) OpportunityDiscoveryResult {
					discoveryOptions := opts.OpportunityDiscoveryOptions
					if discoveryOptions.TempDir == "" {
						discoveryOptions.TempDir = opts.UserTempDiscoveryOptions.TempDir
					}
					if discoveryOptions.Now.IsZero() {
						discoveryOptions.Now = opts.UserTempDiscoveryOptions.Now
					}
					return DiscoverOpportunities(ctx, discoveryOptions)
				}
			}
			discovery := discover(ctx)
			for _, opportunity := range discovery.Opportunities {
				opportunity.Category = normalizedOpportunityCategory(opportunity.Category)
				if optInCategoryEnabled(optInEnabled, opportunity.Category) {
					pathsToProcess = append(pathsToProcess, optInPath{
						path:     opportunity.Path,
						bytes:    opportunity.Bytes,
						category: opportunity.Category,
					})
				}
			}
		}

		// Process browser cache separately - it needs running app detection and uses individual cache paths
		if optInCategoryEnabled(optInEnabled, OpportunityCategoryBrowserCache) && opts.DetectRunningApplications != nil {
			preStates := opts.DetectRunningApplications(ctx)
			for _, config := range browserCacheConfigs {
				// Check if browser is idle before discovery
				preState, ok := runningApplicationStateFor(preStates, config.application)
				if !ok || preState.State != RunningApplicationStateIdle {
					continue
				}
				discovery := discoverBrowserCache(ctx, config, opts.BrowserCacheDiscoveryOptions, opts.Validator)
				if discovery.suppressed || discovery.diagnostic != nil || discovery.incomplete != nil || discovery.opportunity == nil || browserOpportunityProtected(opts.Validator, *discovery.opportunity) {
					continue
				}
				// Post-check that browser is still idle
				postStates := opts.DetectRunningApplications(ctx)
				postState, ok := runningApplicationStateFor(postStates, config.application)
				if !ok || postState.State != RunningApplicationStateIdle {
					continue
				}
				// Add each individual cache directory
				if discovery.opportunity.BrowserCache != nil {
					for _, profile := range discovery.opportunity.BrowserCache.Profiles {
						for _, cache := range profile.Caches {
							if cache.Bytes > 0 {
								pathsToProcess = append(pathsToProcess, optInPath{
									path:     cache.Path,
									bytes:    cache.Bytes,
									category: OpportunityCategoryBrowserCache,
								})
							}
						}
					}
				}
			}
		}

		// Now process all collected paths - check protection and capacity first
		var optInCandidates []delete.Candidate
		categoryByPath := make(map[string]string)
		bytesByPath := make(map[string]int64)

		for _, p := range pathsToProcess {
			if opts.Validator.IsUserProtected(p.path) {
				continue
			}
			// Check Recycle Bin capacity
			cfg, err := probe(p.path)
			if err != nil {
				result.Skipped = append(result.Skipped, SkippedItem{
					Path:   p.path,
					Bytes:  p.bytes,
					Rule:   p.category,
					Reason: issue(recycleBinCapacityProbeFailedIssueCode, "Recycle Bin capacity check failed; skipping rather than risking permanent deletion", true, p.path, p.category),
				})
				continue
			}
			if cfg.NukeOnDelete {
				result.Skipped = append(result.Skipped, SkippedItem{
					Path:   p.path,
					Bytes:  p.bytes,
					Rule:   p.category,
					Reason: issue(recycleBinDisabledIssueCode, "Recycle Bin is disabled for this volume; items would be permanently deleted", true, p.path, p.category),
				})
				continue
			}
			if p.bytes > cfg.MaxCapacity {
				result.Skipped = append(result.Skipped, SkippedItem{
					Path:   p.path,
					Bytes:  p.bytes,
					Rule:   p.category,
					Reason: issue(recycleBinCapacityIssueCode, "Item exceeds Recycle Bin capacity for this volume", true, p.path, p.category),
				})
				continue
			}
			optInCandidates = append(optInCandidates, delete.Candidate{
				Path:  p.path,
				Bytes: p.bytes,
			})
			categoryByPath[p.path] = p.category
			bytesByPath[p.path] = p.bytes
		}

		// Execute all opt-in candidates
		if len(optInCandidates) > 0 {
			optInDeleteResult := delete.ExecuteWithValidator(ctx, optInCandidates, adapter, opts.Validator)
			for _, item := range optInDeleteResult.Deleted {
				category := categoryByPath[item.Path]
				result.Deleted = append(result.Deleted, DeletedItem{
					Path:    item.Path,
					Bytes:   item.Bytes,
					Rule:    category,
					IsOptIn: true,
				})
			}
			for _, item := range optInDeleteResult.Skipped {
				category := categoryByPath[item.Path]
				result.Skipped = append(result.Skipped, SkippedItem{
					Path:   item.Path,
					Bytes:  item.Bytes,
					Rule:   category,
					Reason: issue(item.Reason.Code, item.Reason.Message, true, item.Path, category),
				})
			}
		}
	}

	result.ElapsedMS = time.Since(start).Milliseconds()
	result.Totals = totals(result)
	recordHistorySession(ctx, opts, result, start, time.Now())
	return result
}

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
			OptInDeletedCount:        result.Totals.OptInDeletedCount,
			OptInAffectedBytes:       result.Totals.OptInAffectedBytes,
			AffectedBytes:            result.Totals.AffectedBytes,
		},
	}
	_ = opts.HistoryRecorder.Record(ctx, session, historyItems(session.ID, result))
}

func withoutOpportunityReviewData(result Result) Result {
	result.RunningApplications = nil
	if len(result.IncompleteOpportunityInspections) == 0 {
		result.Errors = withoutRunningApplicationDiagnostics(result.Errors)
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
		if issue.Code == runningApplicationDetectionIssueCode {
			continue
		}
		errorsForHistory = append(errorsForHistory, issue)
	}
	result.Opportunities = nil
	result.IncompleteOpportunityInspections = nil
	result.Errors = errorsForHistory
	return result
}

func withoutRunningApplicationDiagnostics(issues []StructuredIssue) []StructuredIssue {
	if len(issues) == 0 {
		return issues
	}
	filtered := make([]StructuredIssue, 0, len(issues))
	for _, issue := range issues {
		if issue.Code == runningApplicationDetectionIssueCode {
			continue
		}
		filtered = append(filtered, issue)
	}
	return filtered
}

func runningApplicationUnknownIssue(state RunningApplicationState) StructuredIssue {
	message := state.Message
	if message == "" {
		message = applicationDisplayName(state.Application) + " process state could not be determined; browser review was skipped."
	}
	return issue(runningApplicationDetectionIssueCode, message, true, "", "browser_review")
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
			builder.WriteString(fmt.Sprintf("    category: %s\n", normalizedOpportunityCategory(opportunity.Category)))
			builder.WriteString(fmt.Sprintf("    bytes: %d\n", opportunity.Bytes))
			if normalizedOpportunityCategory(opportunity.Category) == OpportunityCategoryUserTemp {
				builder.WriteString(fmt.Sprintf("    latest modified: %s\n", opportunity.LatestModifiedAt.UTC().Format(time.RFC3339)))
				builder.WriteString(fmt.Sprintf("    idle days: %d\n", opportunity.IdleDays))
			}
			if opportunity.BrowserCache != nil {
				builder.WriteString(fmt.Sprintf("    browser: %s\n", applicationDisplayName(opportunity.BrowserCache.Browser)))
				builder.WriteString(fmt.Sprintf("    profiles: %d\n", opportunity.BrowserCache.ProfileCount))
				for _, profile := range opportunity.BrowserCache.Profiles {
					builder.WriteString(fmt.Sprintf("    profile: %s\n", profile.ID))
					if profile.Name != "" {
						builder.WriteString(fmt.Sprintf("      name: %s\n", profile.Name))
					}
					builder.WriteString(fmt.Sprintf("      path: %s\n", profile.Path))
					for _, cache := range profile.Caches {
						builder.WriteString(fmt.Sprintf("      cache: %s\n", cache.Kind))
						builder.WriteString(fmt.Sprintf("        path: %s\n", cache.Path))
						builder.WriteString(fmt.Sprintf("        bytes: %d\n", cache.Bytes))
					}
				}
			}
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

// NormalizedOptInSet returns the set of opt-in categories enabled, resolving
// "all" to all implemented categories and "dev-caches" to all dev caches.
// Returns the set, a list of invalid names (if any), and the list of valid
// names for error reporting.
func NormalizedOptInSet(optIn []string) (enabled map[string]bool, invalid []string, valid []string) {
	opportunityCategories := []string{
		OpportunityCategoryUserTemp,
		OpportunityCategoryCrashDumps,
		OpportunityCategoryWindowsErrorReporting,
		OpportunityCategoryExplorerThumbnailCache,
		OpportunityCategoryINetCache,
		OpportunityCategoryD3DShaderCache,
		OpportunityCategoryNVIDIADXCache,
		OpportunityCategoryBrowserCache,
	}
	devCacheCategories := []string{
		DevCacheCategoryNPM,
		DevCacheCategoryGo,
		DevCacheCategoryPip,
		DevCacheCategoryCargo,
		DevCacheCategoryNuGet,
		DevCacheCategoryCorepack,
	}
	valid = make([]string, 0, len(opportunityCategories)+len(devCacheCategories)+2)
	valid = append(valid, opportunityCategories...)
	valid = append(valid, devCacheCategories...)
	valid = append(valid, DevCacheCategoryAll, "all")

	enabled = make(map[string]bool)
	seen := make(map[string]bool)
	all := false
	devCaches := false
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
			devCaches = true
			continue
		}
		// Check if name is any valid category
		validName := false
		for _, v := range valid {
			if v == name && v != "all" && v != DevCacheCategoryAll {
				enabled[name] = true
				validName = true
				break
			}
		}
		if !validName {
			invalid = append(invalid, name)
		}
	}
	if all {
		// Enable all opportunity categories and all dev caches
		for _, v := range opportunityCategories {
			enabled[v] = true
		}
		for _, v := range devCacheCategories {
			enabled[v] = true
		}
	} else if devCaches {
		// Enable all dev caches
		for _, v := range devCacheCategories {
			enabled[v] = true
		}
	}
	return enabled, invalid, valid
}

func optInCategoryEnabled(enabled map[string]bool, category string) bool {
	return enabled[category]
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
	var optInAffectedBytes int64
	var optInDeletedCount int
	for _, deleted := range result.Deleted {
		affectedBytes += deleted.Bytes
		if deleted.IsOptIn {
			optInAffectedBytes += deleted.Bytes
			optInDeletedCount++
		}
	}
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
		SkippedCount:             len(result.Skipped),
		OpportunityCount:         len(result.Opportunities),
		CandidateBytes:           bytes,
		OpportunityObservedBytes: opportunityBytes,
		OptInCandidateCount:      len(result.OptInCandidates),
		OptInReclaimableBytes:    optInReclaimableBytes,
		OptInDeletedCount:        optInDeletedCount,
		OptInAffectedBytes:       optInAffectedBytes,
		AffectedBytes:            affectedBytes,
	}
}
