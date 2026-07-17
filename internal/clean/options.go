package clean

import (
	"context"

	"github.com/CoreyLyn/Foal/internal/core/delete"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/history"
)

// plannedRecycleBinAction is the stable Recycle Bin action string. Catalog
// ownership is the source of planned actions; this alias remains for actual
// Recycle Bin execution outcomes and label helpers.
const plannedRecycleBinAction = string(DeletionActionMoveToRecycleBin)

// Dev cache categories - these are Review suggestions that become opt-in candidates
const (
	DevCacheCategoryNPM                 = "npm-cache"
	DevCacheCategoryPNPM                = "pnpm-cache"
	DevCacheCategoryYarn                = "yarn-cache"
	DevCacheCategoryGo                  = "go-cache"
	DevCacheCategoryGoModCache          = "go-modcache"
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
	DevCacheCategoryVisualStudioCaches  = "visual-studio-caches"
	DevCacheCategoryAll                 = "dev-caches"
	// CLIAgentCategoryGroup is a selection-group token that expands to
	// independently registered product-scoped CLI-agent categories in catalog
	// order. It owns no resolver, candidates, or deletion action.
	CLIAgentCategoryGroup = "cli-agents"
	// CategoryGrokBuildUpdateResidue is defined in grok_build_update_residue.go
	// (product-scoped CLI-agent residue; not a dev-caches member).
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

// PermanentIdentityCandidate is the fresh candidate context a category-owned
// pre-mutation validator may inspect. It is not an authorization token and does
// not expand the candidate set.
type PermanentIdentityCandidate struct {
	Path     string
	Bytes    int64
	Category string
	// residueDiscovery carries optional env/clock injects for residue
	// revalidation (package-internal; set by shared Execute from Options).
	residueDiscovery GrokBuildResidueDiscoveryOptions
}

// PermanentIdentityValidator re-checks that a permanent candidate still has the
// category-specific identity that authorized discovery (for example exact
// basename under an exact direct parent). It must not mutate the filesystem or
// own permanent removal. ok=false yields a recoverable skip with the original
// category, bytes, and delete_permanently planned action.
type PermanentIdentityValidator func(candidate PermanentIdentityCandidate) (pathsafe.Reason, bool)

// ExecutionPhase identifies an observation-only stage of shared Clean execution.
type ExecutionPhase string

const (
	ExecutionPhaseScanning             ExecutionPhase = "fresh_candidate_scanning"
	ExecutionPhaseRecycleBinSafety     ExecutionPhase = "aggregate_recycle_bin_safety_checks"
	ExecutionPhaseRecycleBinOperations ExecutionPhase = "recycle_bin_operations"
	ExecutionPhasePermanentOperations  ExecutionPhase = "permanent_deletion_operations"
	ExecutionPhaseComplete             ExecutionPhase = "completion"
)

// ExecutionProgress is deliberately absent from Result and its JSON contract.
// A reporter observes execution; it never supplies candidates or safety input.
type ExecutionProgress struct {
	Phase ExecutionPhase
}

type ProgressReporter func(ExecutionProgress)

// Options is the public composition surface for DryRun and Execute.
//
// Required for production composition (CLI / TUI handoff):
//   - Validator, ProtectionDiagnostics / ProtectionLoadError
//   - Plan or OptIn, AllowPermanentDeletion
//   - HistoryRecorder, CommandParameters (optional but usual)
//   - ProgressReporter (TUI observation only)
//
// Adapters and capacity probe: production may leave nil for defaults.
//
// Discovery injects (UserTemp / Opportunities / ApplicationCaches / Review
// suggestions / running-application detection / DevCache resolvers /
// CategoryPlannedActions) are primarily test and gated-execute seams. They
// remain on Options so existing call sites and package-local tests stay stable;
// they do not expand the default candidate set by themselves.
type Options struct {
	// --- composition / safety ---
	Rules                 []Rule
	Validator             pathsafe.Validator
	ProtectionDiagnostics []ProtectionDiagnostic
	ProtectionLoadError   *StructuredIssue

	// --- mutation adapters (nil → production defaults) ---
	RecycleBinAdapter delete.Adapter
	// PermanentRemover permanently deletes authorized candidates. Production
	// uses ordinary filesystem removal; tests inject recording/failing removers.
	// Nil selects delete.FilesystemPermanentRemover.
	PermanentRemover        delete.PermanentRemover
	RecycleBinCapacityProbe RecycleBinCapacityProbe
	// PermanentIdentityValidators optionally re-checks category-owned identity
	// immediately before each permanent removal, after PathSafe validation.
	// Keyed by category/rule ID. Missing or nil entry means PathSafe-only for
	// that category (existing permanent categories need no registration).
	// Validators must not mutate or own the remover. Rejection is a recoverable
	// skip that preserves delete_permanently planned action and never falls
	// back to the Recycle Bin. Production categories may also register via the
	// package-private permanent identity registry; Options injects override tests.
	PermanentIdentityValidators map[string]PermanentIdentityValidator

	// --- history / detailed list ---
	HistoryRecorder   history.Recorder
	DetailedListDir   string
	CommandParameters history.CommandParameters

	// --- category selection ---
	// OptIn holds CLI opt-in tokens (canonical IDs, aliases, or group tokens).
	// Used only when Plan is nil to compile an additive category plan.
	OptIn []string
	// Plan is the optional compiled category selection. When set, DryRun and
	// Execute honor it directly: additive plans always include defaults, exact
	// plans omit unlisted defaults. When nil, an additive plan is compiled from
	// OptIn so existing CLI callers stay compatible.
	Plan *CategoryPlan
	// AllowPermanentDeletion is explicit per-run authorization for candidates
	// whose planned action is delete_permanently. Independent of ordinary
	// execute confirmation. When false, permanent candidates are skipped with
	// permanent_deletion_not_authorized and Recycle Bin work continues.
	AllowPermanentDeletion bool
	// CategoryPlannedActions injects planned actions by category ID for tests.
	// Production leaves it nil so the catalog remains the sole source of truth.
	// Used to exercise permanent deletion without activating production rules.
	CategoryPlannedActions map[string]DeletionAction

	// --- observation ---
	ProgressReporter ProgressReporter

	// --- discovery injects (tests + gated surfaces; production often nil) ---
	UserTempDiscoveryOptions         UserTempDiscoveryOptions
	DiscoverUserTempOpportunities    func(context.Context) UserTempDiscoveryResult
	OpportunityDiscoveryOptions      OpportunityDiscoveryOptions
	DiscoverOpportunities            func(context.Context) OpportunityDiscoveryResult
	BrowserCacheDiscoveryOptions     BrowserCacheDiscoveryOptions
	ApplicationCacheDiscoveryOptions ApplicationCacheDiscoveryOptions
	DiscoverApplicationCaches        DiscoverApplicationCachesFunc
	DiscoverReviewSuggestions        func(context.Context) []ReviewSuggestion
	DetectRunningApplications        func(context.Context) []RunningApplicationState
	DevCachePathResolver             DevCachePathResolver
	// DevCacheRootScopeResolver injects product-scoped root scopes. When set,
	// it takes precedence over DevCachePathResolver. Production leaves it nil.
	DevCacheRootScopeResolver DevCacheRootScopeResolver
	// DevCacheChildDiscoverer overrides catalog-owned structured child discovery
	// for tests. Production leaves it nil so categories use private catalog policy.
	DevCacheChildDiscoverer DevCacheChildDiscoverer
	// GrokBuildResidueDiscoveryOptions injects env/home/clock seams for the
	// grok-build-update-residue category. Production leaves the zero value.
	// Tests must use isolated roots and never read the real user .grok profile.
	GrokBuildResidueDiscoveryOptions GrokBuildResidueDiscoveryOptions
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
	Status                string                 `json:"status"`
	Mode                  string                 `json:"mode"`
	DefaultRuleCatalog    []RuleSummary          `json:"default_rule_catalog"`
	ProtectionRules       []ProtectionRule       `json:"protection_rules"`
	ProtectionDiagnostics []ProtectionDiagnostic `json:"protection_diagnostics"`
	Candidates            []CandidatePreview     `json:"candidates"`
	Deleted               []DeletedItem          `json:"deleted"`
	// Failed holds permanent-deletion candidates that errored after mutation
	// may have begun (permanent_delete_failed). Pre-mutation rejects stay in Skipped.
	Failed                           []FailedItem                      `json:"failed"`
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
	Path          string          `json:"path"`
	Bytes         int64           `json:"bytes"`
	Rule          string          `json:"rule"`
	PlannedAction string          `json:"planned_action,omitempty"`
	Reason        StructuredIssue `json:"reason"`
}

type DeletedItem struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Rule  string `json:"rule"`
	// Action is the actual deletion action taken for a successful item.
	Action  string `json:"action,omitempty"`
	IsOptIn bool   `json:"is_opt_in,omitempty"`
}

// FailedItem is a permanent-deletion candidate that failed after mutation may
// have begun. It is not a pre-mutation skip and never falls back to the Recycle Bin.
type FailedItem struct {
	Path          string          `json:"path"`
	Bytes         int64           `json:"bytes"`
	Rule          string          `json:"rule"`
	PlannedAction string          `json:"planned_action"`
	// Action is the attempted action (always delete_permanently for this ticket).
	Action string          `json:"action"`
	Reason StructuredIssue `json:"reason"`
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
	ApplicationGoogleChrome               = "google_chrome"
	ApplicationMicrosoftEdge              = "microsoft_edge"
	ApplicationMozillaFirefox             = "mozilla_firefox"
	ApplicationGo                         = "go"
	ApplicationCargo                      = "cargo"
	ApplicationDotNet                     = "dotnet"
	ApplicationNuGet                      = "nuget"
	ApplicationNode                       = "node"
	ApplicationPython                     = "python"
	ApplicationUV                         = "uv"
	ApplicationBun                        = "bun"
	ApplicationVisualStudioCode           = "visual_studio_code"
	ApplicationCursor                     = "cursor"
	// ApplicationVisualStudio is full Visual Studio (devenv.exe), not VS Code.
	ApplicationVisualStudio               = "visual_studio"
	ApplicationIntelliJIDEA               = "intellij_idea"
	ApplicationPyCharm                    = "pycharm"
	ApplicationWebStorm                   = "webstorm"
	ApplicationPhpStorm                   = "phpstorm"
	ApplicationRubyMine                   = "rubymine"
	ApplicationCLion                      = "clion"
	ApplicationDataGrip                   = "datagrip"
	ApplicationDataSpell                  = "dataspell"
	ApplicationGoLand                     = "goland"
	ApplicationRustRover                  = "rustrover"
	ApplicationAqua                       = "aqua"
	ApplicationMPS                        = "mps"
	ApplicationWriterside                 = "writerside"
	ApplicationRider                      = "rider"
	// ApplicationGrokBuild is defined in grok_build_update_residue.go.
	RunningApplicationStateRunning        = RunningApplicationStatus("running")
	RunningApplicationStateIdle           = RunningApplicationStatus("idle")
	RunningApplicationStateUnknown        = RunningApplicationStatus("unknown")
	runningApplicationDetectionIssueCode  = "running_application_detection_unknown"
	recycleBinDisabledIssueCode           = "recycle_bin_disabled"
	recycleBinCapacityIssueCode           = "recycle_bin_capacity"
	recycleBinCapacityProbeFailedIssueCode = "recycle_bin_capacity_probe_failed"
	permanentDeletionNotAuthorizedIssueCode = "permanent_deletion_not_authorized"
	permanentDeleteFailedIssueCode        = "permanent_delete_failed"
	devToolRunningIssueCode               = "dev_tool_running"
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
	// RecycleBinMovedBytes is measured logical content successfully moved to
	// the Recycle Bin. PermanentlyDeletedBytes is measured logical content
	// successfully deleted permanently. AffectedBytes is their sum.
	RecycleBinMovedBytes    int64 `json:"recycle_bin_moved_bytes"`
	PermanentlyDeletedBytes int64 `json:"permanently_deleted_bytes"`
	AffectedBytes           int64 `json:"affected_bytes"`
}
