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
	DevCacheCategoryNPM                 = "npm-cache"
	DevCacheCategoryGo                  = "go-cache"
	DevCacheCategoryPip                 = "pip-cache"
	DevCacheCategoryCargo               = "cargo-cache"
	DevCacheCategoryNuGet               = "nuget-cache"
	DevCacheCategoryNuGetGlobalPackages = "nuget-global-packages"
	DevCacheCategoryCorepack            = "corepack-cache"
	DevCacheCategoryUV                  = "uv-cache"
	DevCacheCategoryBun                 = "bun-cache"
	DevCacheCategoryPlaywright          = "playwright-browsers"
	DevCacheCategoryPuppeteerBrowsers   = "puppeteer-browsers"
	DevCacheCategoryElectron            = "electron-cache"
	DevCacheCategoryJetBrainsIDECaches  = "jetbrains-ide-caches"
	DevCacheCategoryAll                 = "dev-caches"
)

// RecycleBinVolumeConfig holds the Recycle Bin configuration for a volume.
type RecycleBinVolumeConfig struct {
	// Volume is the stable identity of the volume containing the candidate.
	Volume string
	// NukeOnDelete is true if the Recycle Bin is disabled for this volume
	// (items are permanently deleted immediately).
	NukeOnDelete bool
	// MaxCapacity is the maximum size in bytes that can be stored in the
	// Recycle Bin for this volume.
	MaxCapacity int64
	// CurrentUsage is the number of bytes already stored in the Recycle Bin
	// for this volume.
	CurrentUsage int64
}

// RecycleBinCapacityProbe returns the Recycle Bin configuration for the
// volume containing the given path.
type RecycleBinCapacityProbe func(path string) (RecycleBinVolumeConfig, error)

// DevCachePathResolver resolves paths for a developer tool cache.
// The category is one of the DevCacheCategory* constants.
// Returns empty slice if no paths can be resolved from env vars/defaults.
// Prefer DevCacheRootScopeResolver when product-scoped application identities
// must be associated with roots; path-only resolution keeps Application empty.
type DevCachePathResolver func(category string) []string

// DevCacheRootScope is one resolved developer-cache root for a category.
// Path is required. Application optionally associates the root with one logical
// application identity for product-scoped idle-before-and-after gating. Empty
// Application keeps category-wide distinctive-process (or shared-runtime) policy.
// Public Clean results remain category-based; product identity is internal only.
type DevCacheRootScope struct {
	Path        string
	Application string
}

// DevCacheRootScopeResolver injects product-aware root scopes for tests and
// future catalog-owned multi-product categories. When non-nil, it replaces
// DevCachePathResolver for root discovery on that run.
type DevCacheRootScopeResolver func(category string) []DevCacheRootScope

// DevCacheChildDiscoverer is defined in structured_dev_cache.go.

// ExecutionPhase identifies an observation-only stage of shared Clean execution.
type ExecutionPhase string

const (
	ExecutionPhaseScanning             ExecutionPhase = "fresh_candidate_scanning"
	ExecutionPhaseRecycleBinSafety     ExecutionPhase = "aggregate_recycle_bin_safety_checks"
	ExecutionPhaseRecycleBinOperations ExecutionPhase = "recycle_bin_operations"
	ExecutionPhaseComplete             ExecutionPhase = "completion"
)

// ExecutionProgress is deliberately absent from Result and its JSON contract.
// A reporter observes execution; it never supplies candidates or safety input.
type ExecutionProgress struct {
	Phase ExecutionPhase
}

type ProgressReporter func(ExecutionProgress)

type Options struct {
	Rules                            []Rule
	Validator                        pathsafe.Validator
	ProtectionDiagnostics            []ProtectionDiagnostic
	ProtectionLoadError              *StructuredIssue
	RecycleBinAdapter                delete.Adapter
	HistoryRecorder                  history.Recorder
	DetailedListDir                  string
	CommandParameters                history.CommandParameters
	UserTempDiscoveryOptions         UserTempDiscoveryOptions
	DiscoverUserTempOpportunities    func(context.Context) UserTempDiscoveryResult
	OpportunityDiscoveryOptions      OpportunityDiscoveryOptions
	DiscoverOpportunities            func(context.Context) OpportunityDiscoveryResult
	BrowserCacheDiscoveryOptions     BrowserCacheDiscoveryOptions
	ApplicationCacheDiscoveryOptions ApplicationCacheDiscoveryOptions
	DiscoverApplicationCaches        DiscoverApplicationCachesFunc
	DiscoverReviewSuggestions        func(context.Context) []ReviewSuggestion
	DetectRunningApplications        func(context.Context) []RunningApplicationState
	// OptIn holds CLI opt-in tokens (canonical IDs, aliases, or group tokens).
	// Used only when Plan is nil to compile an additive category plan.
	OptIn []string
	// Plan is the optional compiled category selection. When set, DryRun and
	// Execute honor it directly: additive plans always include defaults, exact
	// plans omit unlisted defaults. When nil, an additive plan is compiled from
	// OptIn so existing CLI callers stay compatible.
	Plan                    *CategoryPlan
	RecycleBinCapacityProbe RecycleBinCapacityProbe
	DevCachePathResolver    DevCachePathResolver
	// DevCacheRootScopeResolver injects product-scoped root scopes. When set,
	// it takes precedence over DevCachePathResolver. Production leaves it nil.
	DevCacheRootScopeResolver DevCacheRootScopeResolver
	// DevCacheChildDiscoverer overrides catalog-owned structured child discovery
	// for tests. Production leaves it nil so categories use private catalog policy.
	DevCacheChildDiscoverer DevCacheChildDiscoverer
	ProgressReporter        ProgressReporter
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
	ApplicationGo                          = "go"
	ApplicationCargo                       = "cargo"
	ApplicationDotNet                      = "dotnet"
	ApplicationNuGet                       = "nuget"
	ApplicationNode                        = "node"
	ApplicationPython                      = "python"
	ApplicationUV                          = "uv"
	ApplicationBun                         = "bun"
	ApplicationVisualStudioCode            = "visual_studio_code"
	ApplicationCursor                      = "cursor"
	ApplicationIntelliJIDEA                = "intellij_idea"
	ApplicationPyCharm                     = "pycharm"
	ApplicationWebStorm                    = "webstorm"
	ApplicationPhpStorm                    = "phpstorm"
	ApplicationRubyMine                    = "rubymine"
	ApplicationCLion                       = "clion"
	ApplicationDataGrip                    = "datagrip"
	ApplicationDataSpell                   = "dataspell"
	ApplicationGoLand                      = "goland"
	ApplicationRustRover                   = "rustrover"
	ApplicationAqua                        = "aqua"
	ApplicationMPS                         = "mps"
	ApplicationWriterside                  = "writerside"
	ApplicationRider                       = "rider"
	RunningApplicationStateRunning         = RunningApplicationStatus("running")
	RunningApplicationStateIdle            = RunningApplicationStatus("idle")
	RunningApplicationStateUnknown         = RunningApplicationStatus("unknown")
	runningApplicationDetectionIssueCode   = "running_application_detection_unknown"
	recycleBinDisabledIssueCode            = "recycle_bin_disabled"
	recycleBinCapacityIssueCode            = "recycle_bin_capacity"
	recycleBinCapacityProbeFailedIssueCode = "recycle_bin_capacity_probe_failed"
	devToolRunningIssueCode                = "dev_tool_running"
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
	categoryPlan := effectiveCategoryPlan(opts)
	result := newCleanResultSkeleton("dry_run", opts)
	exactDefaults := categoryPlan.Mode == SelectionModeExact
	appendDefaultCandidates(ctx, opts, planDefaultSet(categoryPlan), exactDefaults, &result)
	if ctx.Err() != nil {
		result.ElapsedMS = time.Since(start).Milliseconds()
		result.Totals = totals(result)
		return result
	}

	// Resolve opt-in candidates once; both modes share this resolution.
	optInPlan := planOptInSet(categoryPlan)
	resolution := resolveOptInCandidates(ctx, opts, optInPlan)
	applyOptInResolution(&result, resolution)

	// Review projection is CLI additive dry-run only: non-opted-in opportunities,
	// suggestions, and browser cache review. Exact plans skip the review fan-out
	// so unselected categories are not implicitly resolved.
	if categoryPlan.Mode != SelectionModeExact {
		applyOptInReviewProjection(ctx, opts, &result, optInPlan, resolution)
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

func applyBrowserCacheReview(ctx context.Context, opts Options, result *Result) {
	preStates := opts.DetectRunningApplications(ctx)
	// Scope to supported browser identities; shared detector noise stays out of
	// running_applications and does not invent diagnostics for unrelated apps.
	browserStates := projectRunningApplicationStates(preStates, browserRunningApplicationIdentities()...)
	for _, state := range browserStates {
		replaceRunningApplicationState(result, state)
		if state.State == RunningApplicationStateUnknown {
			result.Errors = append(result.Errors, runningApplicationUnknownIssue(state))
		}
	}
	gate := runningGate{detect: opts.DetectRunningApplications}
	for _, config := range browserCacheConfigs {
		applyOneBrowserCacheReview(ctx, opts, result, gate, preStates, config)
	}
}

// applyOneBrowserCacheReview is the dry-run review surface for a browser whose
// cache was NOT opted in: it gates discovery on running-application state and,
// when the browser stays idle, reports one Browser cache opportunity
// (ADR-0007). Opted-in browser cache is resolved by the opt-in candidate
// resolver, not here.
func applyOneBrowserCacheReview(ctx context.Context, opts Options, result *Result, gate runningGate, preStates []RunningApplicationState, config browserCacheConfig) {
	if localAppDataDir := browserCacheLocalAppDataDir(opts.BrowserCacheDiscoveryOptions); localAppDataDir != "" {
		suppressed, protectedRulePaths := browserDiscoverySuppressed(browserUserDataRoot(localAppDataDir, config), opts.Validator)
		if suppressed {
			suppressProtectionRules(result, protectedRulePaths)
			return
		}
	}
	outcome := gate.gateBrowser(ctx, config.application, preStates, func() browserCacheDiscoveryResult {
		return discoverBrowserCache(ctx, config, opts.BrowserCacheDiscoveryOptions, opts.Validator)
	})
	if !outcome.preIdle {
		return
	}
	discovery := outcome.discovery
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
	if !outcome.postIdle {
		if outcome.postState != nil {
			replaceRunningApplicationState(result, *outcome.postState)
		}
		if outcome.postDiagnostic != nil {
			result.Errors = append(result.Errors, *outcome.postDiagnostic)
		}
		return
	}
	if discovery.opportunity == nil || browserOpportunityProtected(opts.Validator, *discovery.opportunity) {
		return
	}
	result.Opportunities = append(result.Opportunities, *discovery.opportunity)
}

// applyOptInResolution writes the resolved opt-in candidates and gating
// artifacts onto the result. Both dry-run and execute call this so preview and
// execute surface the same candidates, skips, running states, and diagnostics.
func applyOptInResolution(result *Result, res optInResolution) {
	result.OptInCandidates = append(result.OptInCandidates, res.candidates...)
	result.Skipped = append(result.Skipped, res.skipped...)
	result.RunningApplications = mergeRunningApplicationStates(result.RunningApplications, res.runningStates...)
	result.Errors = append(result.Errors, res.diagnostics...)
	if len(res.suppressedProtectionPaths) > 0 {
		suppressProtectionRules(result, res.suppressedProtectionPaths)
	}
}

// applyOptInReviewProjection is the dry-run-only review surface for categories
// the user did not opt in: non-opted-in opportunities (with incompletes),
// review suggestions (minus opted-in dev caches), and browser cache review.
func applyOptInReviewProjection(ctx context.Context, opts Options, result *Result, plan map[string]bool, resolution optInResolution) {
	optedOutCats := optedOutOpportunityCategories(plan)
	if len(optedOutCats) > 0 {
		discovery := discoverOpportunitiesForCategories(ctx, opts, optedOutCats)
		for _, opportunity := range discovery.Opportunities {
			opportunity.Category = normalizedOpportunityCategory(opportunity.Category)
			if opts.Validator.IsUserProtected(opportunity.Path) {
				continue
			}
			result.Opportunities = append(result.Opportunities, opportunity)
		}
		for _, incomplete := range discovery.Incomplete {
			incomplete.Category = normalizedOpportunityCategory(incomplete.Category)
			if opts.Validator.IsUserProtected(incomplete.Path) {
				continue
			}
			result.IncompleteOpportunityInspections = append(result.IncompleteOpportunityInspections, incomplete)
			result.Errors = append(result.Errors, incomplete.Reason)
		}
	}

	optedInDevCacheIdentities := make(map[string]bool)
	for _, c := range resolution.candidates {
		if isDevCacheCategory(c.Category) {
			optedInDevCacheIdentities[pathsafe.NormalizePathForIdentity(c.Path)] = true
		}
	}
	discoverSuggestions := opts.DiscoverReviewSuggestions
	if discoverSuggestions == nil {
		discoverSuggestions = DiscoverReviewSuggestions
	}
	for _, suggestion := range discoverSuggestions(ctx) {
		if suggestion.CachePath != "" && opts.Validator.IsUserProtected(suggestion.CachePath) {
			continue
		}
		if suggestion.CachePath != "" && optedInDevCacheIdentities[pathsafe.NormalizePathForIdentity(suggestion.CachePath)] {
			continue
		}
		result.ReviewSuggestions = append(result.ReviewSuggestions, suggestion)
	}

	if !plan[OpportunityCategoryBrowserCache] && opts.DetectRunningApplications != nil {
		applyBrowserCacheReview(ctx, opts, result)
	}
	if opts.DetectRunningApplications != nil {
		applyApplicationCacheReview(ctx, opts, result, plan)
	}
}

// applyApplicationCacheReview is the dry-run review surface for non-opted-in
// idle Application cache categories (VS Code, Cursor). Opted-in categories are
// resolved by the opt-in candidate resolver instead.
func applyApplicationCacheReview(ctx context.Context, opts Options, result *Result, plan map[string]bool) {
	var categories []string
	for _, id := range applicationCacheCategoryIDs() {
		if !plan[id] {
			categories = append(categories, id)
		}
	}
	if len(categories) == 0 {
		return
	}
	gate := runningGate{detect: opts.DetectRunningApplications}
	// Share one pre-snapshot across application-cache categories. Browser
	// review may already have appended states; still detect here so application
	// cache review works when browser is opted in or browser review is absent.
	preStates := opts.DetectRunningApplications(ctx)
	// Only surface states for applications we are reviewing to avoid duplicate
	// browser/dev-tool noise when browser review already recorded them.
	for _, category := range categories {
		entry, ok := applicationCacheEntry(category)
		if !ok || len(entry.runningApplications) == 0 {
			continue
		}
		application := entry.runningApplications[0]
		if state, found := runningApplicationStateFor(preStates, application); found {
			replaceRunningApplicationState(result, state)
			if state.State == RunningApplicationStateUnknown {
				result.Errors = append(result.Errors, runningApplicationUnknownIssue(state))
			}
		}
		applyOneApplicationCacheReview(ctx, opts, result, gate, preStates, category, application)
	}
}

func applyOneApplicationCacheReview(
	ctx context.Context,
	opts Options,
	result *Result,
	gate runningGate,
	preStates []RunningApplicationState,
	category, application string,
) {
	policyID, policy, ok := applicationCachePolicyForCategory(category)
	if !ok {
		return
	}
	if roaming := applicationCacheRoamingAppDataDir(opts.ApplicationCacheDiscoveryOptions); roaming != "" {
		userDataRoot := applicationCacheUserDataRoot(roaming, policy)
		if opts.Validator.IsUserProtected(userDataRoot) {
			suppressProtectionRules(result, applicationCacheProtectedRulePaths(userDataRoot, opts.Validator))
			return
		}
	}
	outcome := gate.gateApplicationCache(ctx, application, preStates, func() applicationCacheDiscoveryResult {
		return resolveApplicationCacheDiscovery(ctx, opts, policyID)
	})
	if !outcome.preIdle {
		return
	}
	discovery := outcome.discovery
	if len(discovery.suppressedProtectionPaths) > 0 {
		suppressProtectionRules(result, discovery.suppressedProtectionPaths)
	}
	for _, incomplete := range discovery.incompletes {
		if opts.Validator.IsUserProtected(incomplete.Path) {
			continue
		}
		result.IncompleteOpportunityInspections = append(result.IncompleteOpportunityInspections, incomplete)
		result.Errors = append(result.Errors, incomplete.Reason)
	}
	if discovery.canceled || !outcome.postIdle {
		if outcome.postState != nil {
			replaceRunningApplicationState(result, *outcome.postState)
		}
		if outcome.postDiagnostic != nil {
			result.Errors = append(result.Errors, *outcome.postDiagnostic)
		}
		// Measured roots discarded; incompletes already projected above.
		return
	}
	for _, opportunity := range discovery.opportunities {
		if opts.Validator.IsUserProtected(opportunity.Path) {
			continue
		}
		result.Opportunities = append(result.Opportunities, opportunity)
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

func Execute(ctx context.Context, opts Options) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseScanning)
	if opts.ProtectionLoadError != nil {
		result := protectionLoadFailure("execute", opts, start)
		reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseComplete)
		return result
	}
	categoryPlan := effectiveCategoryPlan(opts)
	result := newCleanResultSkeleton("execute", opts)
	exactDefaults := categoryPlan.Mode == SelectionModeExact
	appendDefaultCandidates(ctx, opts, planDefaultSet(categoryPlan), exactDefaults, &result)

	adapter := opts.RecycleBinAdapter
	if adapter == nil {
		adapter = delete.WindowsRecycleBinAdapter{}
	}

	executionCandidates := make([]recycleBinExecutionCandidate, 0, len(result.Candidates))
	for _, candidate := range result.Candidates {
		executionCandidates = append(executionCandidates, recycleBinExecutionCandidate{
			candidate: delete.Candidate{Path: candidate.Path, Bytes: candidate.Bytes},
			rule:      candidate.Rule,
		})
	}

	// Opt-in candidates: resolve fresh, then capacity-check and delete. The
	// resolver scans only planned opt-in categories (ADR-0008) and runs gating;
	// both modes share it, so execute surfaces browser running states and
	// diagnostics. Exact plans omit unlisted defaults and unselected opt-ins.
	optInPlan := planOptInSet(categoryPlan)
	if len(optInPlan) > 0 {
		resolution := resolveOptInCandidates(ctx, opts, optInPlan)
		result.RunningApplications = mergeRunningApplicationStates(result.RunningApplications, resolution.runningStates...)
		result.Errors = append(result.Errors, resolution.diagnostics...)
		result.Skipped = append(result.Skipped, resolution.skipped...)

		for _, c := range resolution.candidates {
			if opts.Validator.IsUserProtected(c.Path) {
				continue
			}
			executionCandidates = append(executionCandidates, recycleBinExecutionCandidate{
				candidate: delete.Candidate{Path: c.Path, Bytes: c.Bytes},
				rule:      c.Category,
				isOptIn:   true,
			})
		}
	}

	reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseRecycleBinSafety)
	executionGroups := prepareRecycleBinCandidateGroups(opts, executionCandidates)
	reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseRecycleBinOperations)
	executeRecycleBinCandidateGroups(ctx, opts, adapter, executionGroups, &result)

	result.ElapsedMS = time.Since(start).Milliseconds()
	result.Totals = totals(result)
	recordHistorySession(ctx, opts, result, start, time.Now())
	reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseComplete)
	return result
}

func reportExecutionProgress(reporter ProgressReporter, phase ExecutionPhase) {
	if reporter == nil {
		return
	}
	defer func() { _ = recover() }()
	reporter(ExecutionProgress{Phase: phase})
}

type recycleBinExecutionCandidate struct {
	candidate delete.Candidate
	rule      string
	isOptIn   bool
}

type recycleBinVolumeGroup struct {
	config     RecycleBinVolumeConfig
	candidates []recycleBinExecutionCandidate
	totalBytes int64
	unsafe     bool
}

type recycleBinVolumeIdentity struct {
	key   string
	known bool
}

type recycleBinCandidateGroups struct {
	byVolume map[string]*recycleBinVolumeGroup
	order    []string
}

func prepareRecycleBinCandidateGroups(opts Options, candidates []recycleBinExecutionCandidate) recycleBinCandidateGroups {
	probe := opts.RecycleBinCapacityProbe
	if probe == nil {
		probe = RecycleBinVolumeCapacity
	}

	groups := make(map[string]*recycleBinVolumeGroup)
	volumeOrder := make([]string, 0)
	for _, candidate := range candidates {
		cfg, err := probe(candidate.candidate.Path)
		identity := recycleBinIdentity(cfg)
		groupKey := identity.key
		if !identity.known {
			groupKey = strings.ToLower(candidate.candidate.Path)
		}
		group, ok := groups[groupKey]
		if !ok {
			group = &recycleBinVolumeGroup{config: cfg}
			groups[groupKey] = group
			volumeOrder = append(volumeOrder, groupKey)
		}
		group.candidates = append(group.candidates, candidate)
		if candidate.candidate.Bytes < 0 || group.totalBytes > int64(^uint64(0)>>1)-candidate.candidate.Bytes {
			group.unsafe = true
		} else {
			group.totalBytes += candidate.candidate.Bytes
		}
		if err != nil || !identity.known || cfg.MaxCapacity < 0 || cfg.CurrentUsage < 0 {
			group.unsafe = true
			continue
		}
		if group.config.NukeOnDelete != cfg.NukeOnDelete || group.config.MaxCapacity != cfg.MaxCapacity || group.config.CurrentUsage != cfg.CurrentUsage {
			group.unsafe = true
		}
	}

	return recycleBinCandidateGroups{byVolume: groups, order: volumeOrder}
}

func executeRecycleBinCandidateGroups(ctx context.Context, opts Options, adapter delete.Adapter, groups recycleBinCandidateGroups, result *Result) {
	for _, volume := range groups.order {
		group := groups.byVolume[volume]
		switch {
		case group.unsafe:
			skipRecycleBinVolume(result, group.candidates, recycleBinCapacityProbeFailedIssueCode, "Recycle Bin capacity state is unknown; skipping this volume rather than risking permanent deletion")
			continue
		case group.config.NukeOnDelete:
			skipRecycleBinVolume(result, group.candidates, recycleBinDisabledIssueCode, "Recycle Bin is disabled for this volume; items would be permanently deleted")
			continue
		case group.config.CurrentUsage > group.config.MaxCapacity || group.totalBytes > group.config.MaxCapacity-group.config.CurrentUsage:
			skipRecycleBinVolume(result, group.candidates, recycleBinCapacityIssueCode, "Selected candidates exceed the remaining Recycle Bin capacity for this volume")
			continue
		}

		deleteCandidates := make([]delete.Candidate, 0, len(group.candidates))
		byPath := make(map[string]recycleBinExecutionCandidate, len(group.candidates))
		for _, candidate := range group.candidates {
			deleteCandidates = append(deleteCandidates, candidate.candidate)
			byPath[candidate.candidate.Path] = candidate
		}
		deleteResult := delete.ExecuteWithValidator(ctx, deleteCandidates, adapter, opts.Validator)
		for _, item := range deleteResult.Deleted {
			candidate := byPath[item.Path]
			result.Deleted = append(result.Deleted, DeletedItem{Path: item.Path, Bytes: item.Bytes, Rule: candidate.rule, IsOptIn: candidate.isOptIn})
		}
		for _, item := range deleteResult.Skipped {
			candidate := byPath[item.Path]
			result.Skipped = append(result.Skipped, SkippedItem{
				Path: item.Path, Bytes: item.Bytes, Rule: candidate.rule,
				Reason: issue(item.Reason.Code, item.Reason.Message, true, item.Path, candidate.rule),
			})
		}
	}
}

func recycleBinIdentity(config RecycleBinVolumeConfig) recycleBinVolumeIdentity {
	key := strings.ToLower(strings.TrimSpace(config.Volume))
	return recycleBinVolumeIdentity{key: key, known: key != ""}
}

func skipRecycleBinVolume(result *Result, candidates []recycleBinExecutionCandidate, code, message string) {
	for _, candidate := range candidates {
		result.Skipped = append(result.Skipped, SkippedItem{
			Path: candidate.candidate.Path, Bytes: candidate.candidate.Bytes, Rule: candidate.rule,
			Reason: issue(code, message, true, candidate.candidate.Path, candidate.rule),
		})
	}
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
	// Cancellation stops remaining cleanup work, but must not erase the outcomes
	// already produced. Persist those outcomes without extending cancellation to
	// the bounded history write.
	_ = opts.HistoryRecorder.Record(context.WithoutCancel(ctx), session, historyItems(session.ID, result))
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
		message = applicationDisplayName(state.Application) + " process state could not be determined; cache review was skipped."
	}
	rule := "cache_review"
	if state.Application == ApplicationGoogleChrome || state.Application == ApplicationMicrosoftEdge {
		rule = "browser_review"
	}
	return issue(runningApplicationDetectionIssueCode, message, true, "", rule)
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

	bytes, err := measureBytes(ctx, path)
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
// "all" to all implemented categories and "dev-caches" to all dev caches.
// Returns the set, a list of invalid names (if any), and the list of valid
// names for error reporting.
func NormalizedOptInSet(optIn []string) (enabled map[string]bool, invalid []string, valid []string) {
	selectable := selectableCategoryIDs()
	// dev-caches expands from catalog policy: developer-cache categories plus
	// idle Application cache opportunities under Developer tools.
	devCaches := developerToolsOptInCategoryIDs()
	valid = make([]string, 0, len(selectable)+2)
	valid = append(valid, selectable...)
	valid = append(valid, DevCacheCategoryAll, "all")

	enabled = make(map[string]bool)
	seen := make(map[string]bool)
	all := false
	allDevCaches := false
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
	} else if allDevCaches {
		for _, v := range devCaches {
			enabled[v] = true
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
