package clean

import (
	"context"
	"fmt"
	"strings"
)

type ReportCategory string

const (
	ReportCategorySystem         ReportCategory = "System"
	ReportCategoryUserEssentials ReportCategory = "User essentials"
	ReportCategoryBrowsers       ReportCategory = "Browsers"
	ReportCategoryDeveloperTools ReportCategory = "Developer tools"
	ReportCategoryProtection     ReportCategory = "Protection"
)

const DefaultCategoryFoalOwnedTempSandboxes = "foal_owned_temp_sandboxes"

type CategoryEligibility string

const (
	CategoryEligibilityDefault            CategoryEligibility = "default"
	CategoryEligibilityOptIn              CategoryEligibility = "opt-in"
	CategoryEligibilityReviewOnly         CategoryEligibility = "review-only"
	CategoryEligibilityPermissionBoundary CategoryEligibility = "permission-boundary"
)

type RunningApplicationPolicy string

const (
	RunningApplicationPolicyNotApplicable              RunningApplicationPolicy = "not-applicable"
	RunningApplicationPolicyBrowserIdleBeforeAfter     RunningApplicationPolicy = "browser-idle-before-and-after-inspection"
	RunningApplicationPolicyApplicationIdleBeforeAfter RunningApplicationPolicy = "application-idle-before-and-after-inspection"
	RunningApplicationPolicyDistinctiveProcessIdle     RunningApplicationPolicy = "distinctive-process-must-be-idle"
	RunningApplicationPolicySharedRuntime              RunningApplicationPolicy = "shared-runtime-not-attributable"
)

// DeletionAction is the stable planned or actual Clean deletion action.
// Catalog ownership: every executable canonical rule must declare exactly one
// of these values. Permanent-delete eligibility is this declaration; there is
// no parallel eligibility boolean.
type DeletionAction string

const (
	DeletionActionMoveToRecycleBin  DeletionAction = "move_to_recycle_bin"
	DeletionActionDeletePermanently DeletionAction = "delete_permanently"
)

// CleanupCategoryDefinition is the path-free policy vocabulary shared by
// Clean callers. Discovery paths and executable configuration stay private to
// the Clean package.
type CleanupCategoryDefinition struct {
	Identifier               string                   `json:"identifier"`
	Label                    string                   `json:"label"`
	ReportCategory           ReportCategory           `json:"report_category"`
	Eligibility              CategoryEligibility      `json:"eligibility"`
	Aliases                  []string                 `json:"aliases"`
	RunningApplicationPolicy RunningApplicationPolicy `json:"running_application_policy"`
	// PlannedAction is required for executable (default/opt-in) categories and
	// must be empty for non-executable entries (permission-boundary, review-only).
	PlannedAction DeletionAction `json:"planned_action,omitempty"`
}

// CleanupCategorySummary is the stable, path-free projection intended for
// read-model consumers such as the Clean TUI.
type CleanupCategorySummary struct {
	Identifier               string                   `json:"identifier"`
	Label                    string                   `json:"label"`
	ReportCategory           ReportCategory           `json:"report_category"`
	Eligibility              CategoryEligibility      `json:"eligibility"`
	RunningApplicationPolicy RunningApplicationPolicy `json:"running_application_policy"`
	// PlannedAction is the catalog-owned deletion action for executable
	// categories. Non-executable summaries omit it (empty).
	PlannedAction DeletionAction `json:"planned_action,omitempty"`
}

// CleanupCategoryCatalog is an immutable metadata catalog. Constructing one
// validates identifiers and aliases so lookup can never be ambiguous.
type CleanupCategoryCatalog struct {
	definitions []CleanupCategoryDefinition
	lookup      map[string]int
}

func (c CleanupCategoryCatalog) Definitions() []CleanupCategoryDefinition {
	definitions := make([]CleanupCategoryDefinition, 0, len(c.definitions))
	for _, definition := range c.definitions {
		aliases := make([]string, len(definition.Aliases))
		copy(aliases, definition.Aliases)
		definition.Aliases = aliases
		definitions = append(definitions, definition)
	}
	return definitions
}

func NewCleanupCategoryCatalog(definitions []CleanupCategoryDefinition) (CleanupCategoryCatalog, error) {
	catalog := CleanupCategoryCatalog{
		definitions: make([]CleanupCategoryDefinition, 0, len(definitions)),
		lookup:      make(map[string]int, len(definitions)),
	}
	for _, definition := range definitions {
		definition.Identifier = strings.ToLower(strings.TrimSpace(definition.Identifier))
		definition.Label = strings.TrimSpace(definition.Label)
		definition.PlannedAction = DeletionAction(strings.TrimSpace(string(definition.PlannedAction)))
		if definition.Identifier == "" || definition.Label == "" || definition.ReportCategory == "" ||
			definition.Eligibility == "" || definition.RunningApplicationPolicy == "" {
			return CleanupCategoryCatalog{}, fmt.Errorf("cleanup category %q has incomplete metadata", definition.Identifier)
		}
		if !validReportCategory(definition.ReportCategory) || !validCategoryEligibility(definition.Eligibility) ||
			!validRunningApplicationPolicy(definition.RunningApplicationPolicy) {
			return CleanupCategoryCatalog{}, fmt.Errorf("cleanup category %q has unsupported metadata", definition.Identifier)
		}
		if isExecutableCategoryEligibility(definition.Eligibility) {
			// Executable rules must declare an explicit supported action. No
			// default is inferred from category, eligibility, caller, or path.
			if definition.PlannedAction == "" {
				return CleanupCategoryCatalog{}, fmt.Errorf("cleanup category %q has incomplete metadata", definition.Identifier)
			}
			if !validDeletionAction(definition.PlannedAction) {
				return CleanupCategoryCatalog{}, fmt.Errorf("cleanup category %q has unsupported metadata", definition.Identifier)
			}
		} else if definition.PlannedAction != "" {
			// Permission-boundary and other non-executable entries remain actionless
			// and cannot enter execution.
			return CleanupCategoryCatalog{}, fmt.Errorf("cleanup category %q has unsupported metadata", definition.Identifier)
		}
		if _, exists := catalog.lookup[definition.Identifier]; exists {
			return CleanupCategoryCatalog{}, fmt.Errorf("duplicate cleanup category identifier or alias %q", definition.Identifier)
		}
		aliases := make([]string, len(definition.Aliases))
		copy(aliases, definition.Aliases)
		definition.Aliases = aliases
		index := len(catalog.definitions)
		catalog.lookup[definition.Identifier] = index
		for aliasIndex, alias := range definition.Aliases {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if alias == "" {
				return CleanupCategoryCatalog{}, fmt.Errorf("cleanup category %q has an empty alias", definition.Identifier)
			}
			if _, exists := catalog.lookup[alias]; exists {
				return CleanupCategoryCatalog{}, fmt.Errorf("duplicate cleanup category identifier or alias %q", alias)
			}
			definition.Aliases[aliasIndex] = alias
			catalog.lookup[alias] = index
		}
		catalog.definitions = append(catalog.definitions, definition)
	}
	return catalog, nil
}

func validReportCategory(category ReportCategory) bool {
	switch category {
	case ReportCategorySystem, ReportCategoryUserEssentials, ReportCategoryBrowsers, ReportCategoryDeveloperTools, ReportCategoryProtection:
		return true
	default:
		return false
	}
}

func validCategoryEligibility(eligibility CategoryEligibility) bool {
	switch eligibility {
	case CategoryEligibilityDefault, CategoryEligibilityOptIn, CategoryEligibilityReviewOnly, CategoryEligibilityPermissionBoundary:
		return true
	default:
		return false
	}
}

func validRunningApplicationPolicy(policy RunningApplicationPolicy) bool {
	switch policy {
	case RunningApplicationPolicyNotApplicable, RunningApplicationPolicyBrowserIdleBeforeAfter,
		RunningApplicationPolicyApplicationIdleBeforeAfter,
		RunningApplicationPolicyDistinctiveProcessIdle, RunningApplicationPolicySharedRuntime:
		return true
	default:
		return false
	}
}

func isExecutableCategoryEligibility(eligibility CategoryEligibility) bool {
	switch eligibility {
	case CategoryEligibilityDefault, CategoryEligibilityOptIn:
		return true
	default:
		return false
	}
}

func validDeletionAction(action DeletionAction) bool {
	switch action {
	case DeletionActionMoveToRecycleBin, DeletionActionDeletePermanently:
		return true
	default:
		return false
	}
}

// InitiallySelectedCategory reports whether the Clean TUI should start with this
// path-free category summary selected. Selection is derived only from shared
// eligibility and planned action: executable defaults and every category whose
// planned action is delete_permanently start selected; non-default Recycle Bin
// opt-ins start unselected. Permission-boundary and other non-executable rows
// never start selected. Callers must not hard-code permanent-category lists.
func InitiallySelectedCategory(summary CleanupCategorySummary) bool {
	switch summary.Eligibility {
	case CategoryEligibilityDefault:
		return true
	case CategoryEligibilityOptIn:
		return summary.PlannedAction == DeletionActionDeletePermanently
	default:
		return false
	}
}

// DeletionActionLabel returns the path-free display label for a planned or
// actual deletion action. Unknown values are returned unchanged.
func DeletionActionLabel(action DeletionAction) string {
	switch action {
	case DeletionActionMoveToRecycleBin:
		return "Recycle Bin"
	case DeletionActionDeletePermanently:
		return "Permanent deletion"
	default:
		return string(action)
	}
}

func (c CleanupCategoryCatalog) Summaries() []CleanupCategorySummary {
	summaries := make([]CleanupCategorySummary, 0, len(c.definitions))
	for _, definition := range c.definitions {
		summaries = append(summaries, summaryFromDefinition(definition))
	}
	return summaries
}

func (c CleanupCategoryCatalog) Summary(identifierOrAlias string) (CleanupCategorySummary, bool) {
	index, ok := c.lookup[strings.ToLower(strings.TrimSpace(identifierOrAlias))]
	if !ok {
		return CleanupCategorySummary{}, false
	}
	return summaryFromDefinition(c.definitions[index]), true
}

func summaryFromDefinition(definition CleanupCategoryDefinition) CleanupCategorySummary {
	return CleanupCategorySummary{
		Identifier:               definition.Identifier,
		Label:                    definition.Label,
		ReportCategory:           definition.ReportCategory,
		Eligibility:              definition.Eligibility,
		RunningApplicationPolicy: definition.RunningApplicationPolicy,
		PlannedAction:            definition.PlannedAction,
	}
}

// supportedApplicationDefinition is the private process-detection definition
// for one logical application. Multiple executable names may represent the
// same application so future tools do not need a new detection switch.
type supportedApplicationDefinition struct {
	id          string
	displayName string
	executables []string
}

// developerApplicationDefinitions is the controlled registry of developer-tool
// process requirements used by DetectSupportedApplications and by developer-
// cache running-application gating. Order is part of the public detection
// surface (browsers are prepended separately; application-cache apps follow).
var developerApplicationDefinitions = []supportedApplicationDefinition{
	{id: ApplicationGo, displayName: "Go", executables: []string{"go.exe"}},
	{id: ApplicationCargo, displayName: "Cargo", executables: []string{"cargo.exe"}},
	{id: ApplicationDotNet, displayName: ".NET", executables: []string{"dotnet.exe"}},
	{id: ApplicationNuGet, displayName: "NuGet", executables: []string{"nuget.exe"}},
	{id: ApplicationNode, displayName: "Node.js", executables: []string{"node.exe"}},
	{id: ApplicationPython, displayName: "Python", executables: []string{"python.exe"}},
	// uv and uvx are one logical application: either process means the tool is running.
	{id: ApplicationUV, displayName: "uv", executables: []string{"uv.exe", "uvx.exe"}},
	// bun and bunx are one logical application: either process means the tool is running.
	{id: ApplicationBun, displayName: "Bun", executables: []string{"bun.exe", "bunx.exe"}},
	// JetBrains IDE launchers: any edition/version of a product shares one identity.
	// 64-bit then 32-bit launcher names match JetBrains Windows bin layout.
	{id: ApplicationIntelliJIDEA, displayName: "IntelliJ IDEA", executables: []string{"idea64.exe", "idea.exe"}},
	{id: ApplicationPyCharm, displayName: "PyCharm", executables: []string{"pycharm64.exe", "pycharm.exe"}},
	{id: ApplicationWebStorm, displayName: "WebStorm", executables: []string{"webstorm64.exe", "webstorm.exe"}},
	{id: ApplicationPhpStorm, displayName: "PhpStorm", executables: []string{"phpstorm64.exe", "phpstorm.exe"}},
	{id: ApplicationRubyMine, displayName: "RubyMine", executables: []string{"rubymine64.exe", "rubymine.exe"}},
	{id: ApplicationCLion, displayName: "CLion", executables: []string{"clion64.exe", "clion.exe"}},
	{id: ApplicationDataGrip, displayName: "DataGrip", executables: []string{"datagrip64.exe", "datagrip.exe"}},
	{id: ApplicationDataSpell, displayName: "DataSpell", executables: []string{"dataspell64.exe", "dataspell.exe"}},
	{id: ApplicationGoLand, displayName: "GoLand", executables: []string{"goland64.exe", "goland.exe"}},
	{id: ApplicationRustRover, displayName: "RustRover", executables: []string{"rustrover64.exe", "rustrover.exe"}},
	{id: ApplicationAqua, displayName: "Aqua", executables: []string{"aqua64.exe", "aqua.exe"}},
	{id: ApplicationMPS, displayName: "MPS", executables: []string{"mps64.exe", "mps.exe"}},
	{id: ApplicationWriterside, displayName: "Writerside", executables: []string{"writerside64.exe", "writerside.exe"}},
	{id: ApplicationRider, displayName: "Rider", executables: []string{"rider64.exe", "rider.exe"}},
	// Full Visual Studio IDE (not VS Code / Cursor). devenv.exe is the
	// distinctive process Microsoft cache-clear guidance requires idle.
	{id: ApplicationVisualStudio, displayName: "Visual Studio", executables: []string{"devenv.exe"}},
	// Grok Build CLI agent: one logical identity covering both updater-managed
	// Windows executables (grok.exe and agent.exe).
	{id: ApplicationGrokBuild, displayName: "Grok Build", executables: []string{"grok.exe", "agent.exe"}},
}

// applicationCacheApplicationDefinitions is the controlled registry of idle
// Application cache owners. Process detection is shared with
// DetectSupportedApplications after developer tools. Each editor is an
// independent logical application so gates never cross-authorize.
var applicationCacheApplicationDefinitions = []supportedApplicationDefinition{
	{id: ApplicationVisualStudioCode, displayName: "Visual Studio Code", executables: []string{"Code.exe"}},
	{id: ApplicationCursor, displayName: "Cursor", executables: []string{"Cursor.exe"}},
	{id: ApplicationVisualStudioCodeInsiders, displayName: "VS Code Insiders", executables: []string{"Code - Insiders.exe"}},
	{id: ApplicationVSCodium, displayName: "VSCodium", executables: []string{"VSCodium.exe"}},
	{id: ApplicationWindsurf, displayName: "Windsurf", executables: []string{"Windsurf.exe"}},
}

// categoryCatalogEntry is the private canonical registration point. Every
// executable category binds one resolver adapter here; callers never dispatch
// by category identifier or combine independent family booleans. Public catalog
// projections expose only path-free definition fields. Developer-cache entries
// additionally bind path/child discovery policy, Review suggestion keys, and
// running applications. Application-cache entries bind an idle-cache policy id.
//
// Path resolvers, allowlists, structural matchers, and executable paths never
// appear on public CleanupCategoryDefinition / CleanupCategorySummary.
type categoryResolverKind string

const (
	categoryResolverDefault              categoryResolverKind = "default"
	categoryResolverExistenceOpportunity categoryResolverKind = "existence-opportunity"
	categoryResolverBrowserCache         categoryResolverKind = "browser-cache"
	categoryResolverApplicationCache     categoryResolverKind = "application-cache"
	categoryResolverDeveloperCache       categoryResolverKind = "developer-cache"
	categoryResolverNonExecutable        categoryResolverKind = "non-executable"
)

type categoryResolver interface {
	resolve(context.Context, Options, string, *categoryCoreResult)
}

type defaultCategoryResolver struct{}
type existenceOpportunityResolver struct{}
type browserCacheResolver struct{}
type applicationCacheResolver struct{}
type developerCacheResolver struct{}

func (defaultCategoryResolver) resolve(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	resolveDefaultCategory(ctx, opts, category, core)
}

func (existenceOpportunityResolver) resolve(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	resolveExistenceOpportunityCategory(ctx, opts, category, core)
}

func (browserCacheResolver) resolve(ctx context.Context, opts Options, _ string, core *categoryCoreResult) {
	resolveBrowserCacheCategory(ctx, opts, core)
}

func (applicationCacheResolver) resolve(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	resolveApplicationCacheCategory(ctx, opts, category, core)
}

func (developerCacheResolver) resolve(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	resolveDeveloperCacheCategory(ctx, opts, category, core)
}

type categorySafetyNoteResolver func(CategoryResolution) string

// existenceRootBase selects the current-user base directory for an existence-
// observed fixed root. Paths are exact allowlisted joins only.
type existenceRootBase string

const (
	existenceRootLocalAppData    existenceRootBase = "local-app-data"
	existenceRootLocalAppDataLow existenceRootBase = "local-app-data-low"
)

// existenceRootSpec is one exact relative root under a known current-user base.
// Missing bases or missing roots are silent absence (no incomplete result).
//
// When matchDirectFileName is non-nil, segments form a parent directory that is
// never itself a candidate. Only direct children whose basenames match the
// predicate and that are regular non-reparse files become independent
// candidates. Missing parent or zero matches is silent empty (no whole-root
// fallback). Non-matching siblings are never candidates.
type existenceRootSpec struct {
	base                existenceRootBase
	segments            []string
	matchDirectFileName func(name string) bool
}

type categoryCatalogEntry struct {
	definition          CleanupCategoryDefinition
	resolverKind        categoryResolverKind
	resolver            categoryResolver
	previewSafetyNote   categorySafetyNoteResolver
	// cliAgentProduct marks an independently registered product-scoped CLI-agent
	// category for the `cli-agents` selection group. The group token expands
	// these entries only; it owns no resolver, candidates, or deletion action.
	// Membership is explicit catalog registration (never inferred from report
	// group, path shape, or cache-like names).
	cliAgentProduct bool
	// existenceRoots lists exact fixed roots for existence-opportunity
	// categories. Empty means no fixed-root discovery (user_temp).
	existenceRoots      []existenceRootSpec
	runningApplications []string
	// resolvePaths resolves env/default roots for a developer-cache category.
	// Required when developerCache is true unless resolveRootScopes is set;
	// ignored when resolveRootScopes is non-nil. Root resolution never
	// authorizes deletion by itself.
	resolvePaths func(devCachePathDependencies) []string
	// resolveRootScopes optionally returns product-scoped roots for a
	// developer-cache category. When set, each scope may associate a logical
	// application identity for independent idle-before-and-after gating. When
	// nil, resolvePaths produces scopes with empty Application (category-wide
	// gate). Public Clean stays category-based.
	resolveRootScopes func(devCachePathDependencies) []DevCacheRootScope
	// discoverChildren is an optional structured child candidate discovery
	// policy. When nil, each resolved root is one Opt-in candidate (whole-root
	// mode). When set, Foal discovers independent child candidates under each
	// unprotected root; the root itself is never a candidate. Policies must
	// fail closed: unknown layouts return no children until explicitly updated.
	discoverChildren func(ctx context.Context, root string) []string
	// reviewSuggestionTools lists Review suggestion allowlist tool keys
	// associated with this developer-cache category. Empty when no suggestion
	// probe is associated (for example cargo). Referenced keys must exist.
	reviewSuggestionTools []string
	// applicationCachePolicyID selects a private applicationCachePolicy for
	// application-cache opportunity categories. Required when applicationCache.
	applicationCachePolicyID string
}

func defaultCategoryEntry(definition CleanupCategoryDefinition) categoryCatalogEntry {
	return categoryCatalogEntry{
		definition:   definition,
		resolverKind: categoryResolverDefault,
		resolver:     defaultCategoryResolver{},
	}
}

func existenceOpportunityEntry(definition CleanupCategoryDefinition, fixedLocalAppDataPath ...string) categoryCatalogEntry {
	var roots []existenceRootSpec
	if len(fixedLocalAppDataPath) > 0 {
		roots = []existenceRootSpec{{
			base:     existenceRootLocalAppData,
			segments: append([]string(nil), fixedLocalAppDataPath...),
		}}
	}
	return existenceOpportunityRootsEntry(definition, roots...)
}

// existenceOpportunityRootsEntry registers an existence-opportunity category
// with one or more exact fixed roots under Local AppData and/or LocalLow.
func existenceOpportunityRootsEntry(definition CleanupCategoryDefinition, roots ...existenceRootSpec) categoryCatalogEntry {
	copied := make([]existenceRootSpec, len(roots))
	for i, root := range roots {
		copied[i] = existenceRootSpec{
			base:                root.base,
			segments:            append([]string(nil), root.segments...),
			matchDirectFileName: root.matchDirectFileName,
		}
	}
	return categoryCatalogEntry{
		definition:     definition,
		resolverKind:   categoryResolverExistenceOpportunity,
		resolver:       existenceOpportunityResolver{},
		existenceRoots: copied,
	}
}

func browserCacheCategoryEntry(definition CleanupCategoryDefinition) categoryCatalogEntry {
	return categoryCatalogEntry{
		definition:   definition,
		resolverKind: categoryResolverBrowserCache,
		resolver:     browserCacheResolver{},
	}
}

func applicationCacheCategoryEntry(
	definition CleanupCategoryDefinition,
	policyID string,
	runningApplications ...string,
) categoryCatalogEntry {
	return categoryCatalogEntry{
		definition:               definition,
		resolverKind:             categoryResolverApplicationCache,
		resolver:                 applicationCacheResolver{},
		applicationCachePolicyID: policyID,
		runningApplications:      append([]string(nil), runningApplications...),
	}
}

func nonExecutableCategoryEntry(definition CleanupCategoryDefinition) categoryCatalogEntry {
	return categoryCatalogEntry{definition: definition, resolverKind: categoryResolverNonExecutable}
}

func withPreviewSafetyNote(entry categoryCatalogEntry, note categorySafetyNoteResolver) categoryCatalogEntry {
	entry.previewSafetyNote = note
	return entry
}

func staticPreviewSafetyNote(note string) categorySafetyNoteResolver {
	return func(CategoryResolution) string { return note }
}

func developerCacheEntry(
	definition CleanupCategoryDefinition,
	resolvePaths func(devCachePathDependencies) []string,
	reviewSuggestionTools []string,
	runningApplications ...string,
) categoryCatalogEntry {
	return categoryCatalogEntry{
		definition:            definition,
		resolverKind:          categoryResolverDeveloperCache,
		resolver:              developerCacheResolver{},
		resolvePaths:          resolvePaths,
		reviewSuggestionTools: append([]string(nil), reviewSuggestionTools...),
		runningApplications:   append([]string(nil), runningApplications...),
	}
}

// developerCacheEntryWithChildren registers a developer-cache category that
// uses structured child candidate discovery under each resolved root instead of
// whole-root candidates. discoverChildren must be non-nil and fail closed.
func developerCacheEntryWithChildren(
	definition CleanupCategoryDefinition,
	resolvePaths func(devCachePathDependencies) []string,
	discoverChildren func(ctx context.Context, root string) []string,
	reviewSuggestionTools []string,
	runningApplications ...string,
) categoryCatalogEntry {
	entry := developerCacheEntry(definition, resolvePaths, reviewSuggestionTools, runningApplications...)
	entry.discoverChildren = discoverChildren
	return entry
}

// developerCacheEntryWithProductScopedChildren registers a developer-cache
// category whose roots are product-scoped (resolveRootScopes) and whose
// candidates are structured children under each root. discoverChildren must be
// non-nil and fail closed. Public Clean remains category-based.
func developerCacheEntryWithProductScopedChildren(
	definition CleanupCategoryDefinition,
	resolveRootScopes func(devCachePathDependencies) []DevCacheRootScope,
	discoverChildren func(ctx context.Context, root string) []string,
	reviewSuggestionTools []string,
	runningApplications ...string,
) categoryCatalogEntry {
	entry := developerCacheEntry(definition, nil, reviewSuggestionTools, runningApplications...)
	entry.resolveRootScopes = resolveRootScopes
	entry.discoverChildren = discoverChildren
	return entry
}

var canonicalCategoryEntries = []categoryCatalogEntry{
	// Complete rule matrix (ADR 0018 / docs/plan/clean-deletion-policy.md):
	// 28 delete_permanently + 6 move_to_recycle_bin + 1 actionless permission boundary.
	// Recycle Bin system opt-ins: user_temp / crash_dumps / WER stay whole-root;
	// explorer_thumbnail_cache and inet_cache use exact research allowlists (#239).
	defaultCategoryEntry(categoryDefinition(DefaultCategoryFoalOwnedTempSandboxes, "Foal-owned temp sandboxes", ReportCategoryUserEssentials, CategoryEligibilityDefault, RunningApplicationPolicyNotApplicable, DeletionActionMoveToRecycleBin)),
	existenceOpportunityEntry(categoryDefinition(OpportunityCategoryUserTemp, "User temp", ReportCategoryUserEssentials, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionMoveToRecycleBin)),
	existenceOpportunityEntry(categoryDefinition(OpportunityCategoryCrashDumps, "Crash dumps", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionMoveToRecycleBin), "CrashDumps"),
	existenceOpportunityEntry(categoryDefinition(OpportunityCategoryWindowsErrorReporting, "Windows Error Reporting", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionMoveToRecycleBin), "Microsoft", "Windows", "WER"),
	// Explorer thumbnail/icon DBs: exact thumbcache_*.db / iconcache_*.db under
	// Local\Microsoft\Windows\Explorer only. Parent Explorer, ETL logs,
	// RecommendationsFilterList.json, nested decoys, and legacy IconCache.db
	// outside Explorer are never candidates. Missing matches ⇒ empty (no
	// whole-root fallback). Evidence: docs/research/explorer-thumbnail-and-inet-cache-allowlists.md (#235/#239).
	existenceOpportunityRootsEntry(
		categoryDefinition(OpportunityCategoryExplorerThumbnailCache, "Explorer thumbnail cache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionMoveToRecycleBin),
		existenceRootSpec{
			base:                existenceRootLocalAppData,
			segments:            []string{"Microsoft", "Windows", "Explorer"},
			matchDirectFileName: isExplorerThumbnailCacheDBName,
		},
	),
	// INetCache: exact Temporary Internet Files dirs IE and Low\IE only.
	// Whole INetCache, Content.IE5 junctions, Low (parent), SuggestedSites,
	// Virtualized, Office Content.*, and thumbnails are never candidates.
	// Evidence: docs/research/explorer-thumbnail-and-inet-cache-allowlists.md (#235/#239).
	existenceOpportunityRootsEntry(
		categoryDefinition(OpportunityCategoryINetCache, "INetCache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionMoveToRecycleBin),
		existenceRootSpec{base: existenceRootLocalAppData, segments: []string{"Microsoft", "Windows", "INetCache", "IE"}},
		existenceRootSpec{base: existenceRootLocalAppData, segments: []string{"Microsoft", "Windows", "INetCache", "Low", "IE"}},
	),
	existenceOpportunityEntry(categoryDefinition(OpportunityCategoryD3DShaderCache, "D3D shader cache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionDeletePermanently), "D3DSCache"),
	existenceOpportunityEntry(categoryDefinition(OpportunityCategoryNVIDIADXCache, "NVIDIA DX cache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionDeletePermanently), "NVIDIA", "DXCache"),
	// AMD GPU/shader caches: exact allowlisted children under Local AMD (+ optional
	// LocalLow AMD\DxCache). Parent AMD and non-allowlisted siblings are never candidates.
	// Evidence: docs/research/amd-intel-gpu-shader-caches.md (#234/#238).
	withPreviewSafetyNote(existenceOpportunityRootsEntry(
		categoryDefinition(OpportunityCategoryAMDGPUShaderCaches, "AMD GPU shader caches", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionDeletePermanently),
		existenceRootSpec{base: existenceRootLocalAppData, segments: []string{"AMD", "DxCache"}},
		existenceRootSpec{base: existenceRootLocalAppData, segments: []string{"AMD", "DxcCache"}},
		existenceRootSpec{base: existenceRootLocalAppData, segments: []string{"AMD", "Dx9Cache"}},
		existenceRootSpec{base: existenceRootLocalAppData, segments: []string{"AMD", "OglCache"}},
		existenceRootSpec{base: existenceRootLocalAppData, segments: []string{"AMD", "VkCache"}},
		existenceRootSpec{base: existenceRootLocalAppDataLow, segments: []string{"AMD", "DxCache"}},
	), staticPreviewSafetyNote(gpuShaderCacheOptInImpactNotice)),
	// Intel GPU shader cache: exact LocalLow Intel\ShaderCache only. Local Intel
	// tree and ProgramData copies are never executable discovery roots.
	withPreviewSafetyNote(existenceOpportunityRootsEntry(
		categoryDefinition(OpportunityCategoryIntelGPUShaderCache, "Intel GPU shader cache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionDeletePermanently),
		existenceRootSpec{base: existenceRootLocalAppDataLow, segments: []string{"Intel", "ShaderCache"}},
	), staticPreviewSafetyNote(gpuShaderCacheOptInImpactNotice)),
	withPreviewSafetyNote(browserCacheCategoryEntry(categoryDefinition(OpportunityCategoryBrowserCache, "Browser cache", ReportCategoryBrowsers, CategoryEligibilityOptIn, RunningApplicationPolicyBrowserIdleBeforeAfter, DeletionActionDeletePermanently)), staticPreviewSafetyNote(browserCacheOptInImpactNotice)),
	withPreviewSafetyNote(applicationCacheCategoryEntry(
		categoryDefinition(
			OpportunityCategoryVSCodeCache,
			"VS Code cache",
			ReportCategoryDeveloperTools,
			CategoryEligibilityOptIn,
			RunningApplicationPolicyApplicationIdleBeforeAfter,
			DeletionActionDeletePermanently,
		),
		applicationCachePolicyVSCode,
		ApplicationVisualStudioCode,
	), applicationCachePreviewSafetyNote),
	withPreviewSafetyNote(applicationCacheCategoryEntry(
		categoryDefinition(
			OpportunityCategoryCursorCache,
			"Cursor cache",
			ReportCategoryDeveloperTools,
			CategoryEligibilityOptIn,
			RunningApplicationPolicyApplicationIdleBeforeAfter,
			DeletionActionDeletePermanently,
		),
		applicationCachePolicyCursor,
		ApplicationCursor,
	), applicationCachePreviewSafetyNote),
	// Additional VS Code-family editors: same regenerating-cache allowlist and
	// independent idle gates. Selecting one never authorizes or suppresses another.
	withPreviewSafetyNote(applicationCacheCategoryEntry(
		categoryDefinition(
			OpportunityCategoryVSCodeInsidersCache,
			"VS Code Insiders cache",
			ReportCategoryDeveloperTools,
			CategoryEligibilityOptIn,
			RunningApplicationPolicyApplicationIdleBeforeAfter,
			DeletionActionDeletePermanently,
		),
		applicationCachePolicyVSCodeInsiders,
		ApplicationVisualStudioCodeInsiders,
	), applicationCachePreviewSafetyNote),
	withPreviewSafetyNote(applicationCacheCategoryEntry(
		categoryDefinition(
			OpportunityCategoryVSCodiumCache,
			"VSCodium cache",
			ReportCategoryDeveloperTools,
			CategoryEligibilityOptIn,
			RunningApplicationPolicyApplicationIdleBeforeAfter,
			DeletionActionDeletePermanently,
		),
		applicationCachePolicyVSCodium,
		ApplicationVSCodium,
	), applicationCachePreviewSafetyNote),
	withPreviewSafetyNote(applicationCacheCategoryEntry(
		categoryDefinition(
			OpportunityCategoryWindsurfCache,
			"Windsurf cache",
			ReportCategoryDeveloperTools,
			CategoryEligibilityOptIn,
			RunningApplicationPolicyApplicationIdleBeforeAfter,
			DeletionActionDeletePermanently,
		),
		applicationCachePolicyWindsurf,
		ApplicationWindsurf,
	), applicationCachePreviewSafetyNote),
	// Package and build caches: proven regenerable roots; env/default
	// resolvers, gates, and impact notices unchanged.
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryNPM, "npm cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime, DeletionActionDeletePermanently),
		resolveNPMCachePaths,
		[]string{"npm"},
	),
	// pnpm content-addressable store: shared-runtime (Node hosts pnpm). Whole
	// store root only; never project node_modules or virtual stores.
	withPreviewSafetyNote(developerCacheEntry(
		categoryDefinition(DevCacheCategoryPNPM, "pnpm store", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime, DeletionActionDeletePermanently),
		resolvePNPMCachePaths,
		[]string{"pnpm"},
	), staticPreviewSafetyNote(pnpmCacheOptInImpactNotice)),
	// yarn global cache: shared-runtime (Node hosts yarn). Classic Windows
	// Yarn\Cache root or YARN_CACHE_FOLDER; never project .yarn/cache.
	withPreviewSafetyNote(developerCacheEntry(
		categoryDefinition(DevCacheCategoryYarn, "yarn cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime, DeletionActionDeletePermanently),
		resolveYarnCachePaths,
		[]string{"yarn"},
	), staticPreviewSafetyNote(yarnCacheOptInImpactNotice)),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryGo, "Go build cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle, DeletionActionDeletePermanently),
		resolveGoCachePaths,
		[]string{"go"},
		ApplicationGo,
	),
	// Go module download cache: separate from go-cache (GOCACHE / go-build).
	// Distinctive-process idle gate shares ApplicationGo with the build cache.
	withPreviewSafetyNote(developerCacheEntry(
		categoryDefinition(DevCacheCategoryGoModCache, "Go module cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle, DeletionActionDeletePermanently),
		resolveGoModCachePaths,
		[]string{"go"},
		ApplicationGo,
	), staticPreviewSafetyNote(goModCacheOptInImpactNotice)),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryPip, "pip cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime, DeletionActionDeletePermanently),
		resolvePipCachePaths,
		[]string{"pip"},
	),
	// Cargo registry cache + unpacked sources: multi-root allowlist under
	// CARGO_HOME (registry/cache, registry/src only). Distinctive-process idle
	// gate (cargo.exe). Permanent; re-fetch impact disclosed.
	withPreviewSafetyNote(developerCacheEntry(
		categoryDefinition(DevCacheCategoryCargo, "Cargo cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle, DeletionActionDeletePermanently),
		resolveCargoCachePaths,
		nil,
		ApplicationCargo,
	), staticPreviewSafetyNote(cargoCacheOptInImpactNotice)),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryNuGet, "NuGet cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle, DeletionActionDeletePermanently),
		resolveNuGetCachePaths,
		[]string{"dotnet"},
		ApplicationDotNet, ApplicationNuGet,
	),
	withPreviewSafetyNote(developerCacheEntry(
		categoryDefinition(DevCacheCategoryNuGetGlobalPackages, "NuGet global packages", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle, DeletionActionDeletePermanently),
		resolveNuGetGlobalPackagesPaths,
		[]string{"dotnet"},
		ApplicationDotNet, ApplicationNuGet,
	), staticPreviewSafetyNote(nugetGlobalPackagesOptInImpactNotice)),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryCorepack, "Corepack cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime, DeletionActionDeletePermanently),
		resolveCorepackOptInCachePaths,
		[]string{"corepack"},
	),
	withPreviewSafetyNote(developerCacheEntry(
		categoryDefinition(DevCacheCategoryUV, "uv cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle, DeletionActionDeletePermanently),
		resolveUVCachePaths,
		[]string{"uv"},
		ApplicationUV,
	), staticPreviewSafetyNote(uvCacheOptInImpactNotice)),
	withPreviewSafetyNote(developerCacheEntry(
		categoryDefinition(DevCacheCategoryBun, "Bun cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle, DeletionActionDeletePermanently),
		resolveBunCachePaths,
		[]string{"bun"},
		ApplicationBun,
	), staticPreviewSafetyNote(bunCacheOptInImpactNotice)),
	// Playwright global browser binaries: structured child discovery under the
	// env/default root. Shared-runtime policy: do not attribute node/chrome/etc.
	// as Playwright-owned. Permanent deletion; no Review suggestion probe.
	withPreviewSafetyNote(developerCacheEntryWithChildren(
		categoryDefinition(DevCacheCategoryPlaywright, "Playwright browsers", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime, DeletionActionDeletePermanently),
		resolvePlaywrightBrowserPaths,
		discoverPlaywrightBrowserChildren,
		nil,
	), staticPreviewSafetyNote(playwrightBrowsersOptInImpactNotice)),
	// Puppeteer browser binaries: shared-runtime (Node/Python/Chrome/Firefox are
	// not attributable). Structured children under env/default root only; root
	// and product parents are never candidates (ADR 0011). Permanent deletion.
	withPreviewSafetyNote(developerCacheEntryWithChildren(
		categoryDefinition(DevCacheCategoryPuppeteerBrowsers, "Puppeteer browsers", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime, DeletionActionDeletePermanently),
		resolvePuppeteerCachePaths,
		discoverPuppeteerBrowserChildren,
		nil,
	), staticPreviewSafetyNote(puppeteerBrowsersOptInImpactNotice)),
	// Electron download cache: whole-root under env/default only. Shared-runtime
	// (Node/Electron hosts are not attributable). Permanent deletion; no
	// Review suggestion probe and no invented Electron cleanup command.
	withPreviewSafetyNote(developerCacheEntry(
		categoryDefinition(DevCacheCategoryElectron, "Electron cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime, DeletionActionDeletePermanently),
		resolveElectronCachePaths,
		nil,
	), staticPreviewSafetyNote(electronCacheOptInImpactNotice)),
	// JetBrains IDE system caches: product-scoped roots under %LOCALAPPDATA%\JetBrains
	// with structured children (caches/index; Rider also resharper-host). Distinctive-
	// process policy; each product identity gates independently via
	// DevCacheRootScope.Application. Permanent deletion; no Review probe.
	withPreviewSafetyNote(developerCacheEntryWithProductScopedChildren(
		categoryDefinition(DevCacheCategoryJetBrainsIDECaches, "JetBrains IDE caches", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle, DeletionActionDeletePermanently),
		resolveJetBrainsIDECacheRootScopes,
		discoverJetBrainsIDECacheChildren,
		nil,
		// Application list must stay aligned with jetbrainsIDEProductPolicies.
		ApplicationIntelliJIDEA, ApplicationPyCharm,
		ApplicationWebStorm, ApplicationPhpStorm, ApplicationRubyMine,
		ApplicationCLion, ApplicationDataGrip, ApplicationDataSpell,
		ApplicationGoLand, ApplicationRustRover, ApplicationAqua,
		ApplicationMPS, ApplicationWriterside, ApplicationRider,
	), staticPreviewSafetyNote(jetbrainsIDECachesOptInImpactNotice)),
	// Visual Studio regenerable caches: one product-scoped parent under
	// %LOCALAPPDATA%\Microsoft\VisualStudio with exact ComponentModelCache
	// (per 14.0+ instance hive) and shared Roslyn children. Distinctive-process
	// idle on devenv.exe; independent of VS Code/Cursor. Permanent deletion;
	// no Review probe, no ProgramData/settings/solutions.
	withPreviewSafetyNote(developerCacheEntryWithProductScopedChildren(
		categoryDefinition(DevCacheCategoryVisualStudioCaches, "Visual Studio caches", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle, DeletionActionDeletePermanently),
		resolveVisualStudioCacheRootScopes,
		discoverVisualStudioCacheChildren,
		nil,
		ApplicationVisualStudio,
	), staticPreviewSafetyNote(visualStudioCachesOptInImpactNotice)),
	// Grok Build updater residue: exact ordinary .old backups under $GROK_HOME\bin.
	// Product-scoped dedicated resolver (not developer-cache / not dev-caches).
	// Distinctive-process idle on grok.exe|agent.exe; permanent; witnesses never candidates.
	// Evidence: ADR 0021/0022, docs/plan/grok-build-update-residue.md.
	withPreviewSafetyNote(grokBuildUpdateResidueCategoryEntry(
		categoryDefinition(
			CategoryGrokBuildUpdateResidue,
			"Grok Build update residue",
			ReportCategoryDeveloperTools,
			CategoryEligibilityOptIn,
			RunningApplicationPolicyDistinctiveProcessIdle,
			DeletionActionDeletePermanently,
		),
	), staticPreviewSafetyNote(grokBuildUpdateResidueOptInImpactNotice)),
	// Non-executable permission boundary: actionless and cannot enter execution.
	nonExecutableCategoryEntry(categoryDefinition("administrator_only_caches", "Administrator-only caches", ReportCategorySystem, CategoryEligibilityPermissionBoundary, RunningApplicationPolicyNotApplicable, "")),
}

func categoryDefinition(identifier, label string, reportCategory ReportCategory, eligibility CategoryEligibility, runningPolicy RunningApplicationPolicy, plannedAction DeletionAction) CleanupCategoryDefinition {
	return CleanupCategoryDefinition{
		Identifier:               identifier,
		Label:                    label,
		ReportCategory:           reportCategory,
		Eligibility:              eligibility,
		Aliases:                  []string{},
		RunningApplicationPolicy: runningPolicy,
		PlannedAction:            plannedAction,
	}
}

var canonicalCleanupCategoryCatalog = mustCleanupCategoryCatalog(canonicalCategoryEntries)

func init() {
	if err := validateCategoryResolverRegistry(canonicalCategoryEntries); err != nil {
		panic(err)
	}
	// Private developer-cache validation runs after package-level vars (including
	// the Review suggestion allowlist) are initialized.
	if err := validateDeveloperCacheRegistry(canonicalCategoryEntries, developerApplicationDefinitions, reviewSuggestionAllowlist); err != nil {
		panic(err)
	}
	if err := validateApplicationCacheRegistry(canonicalCategoryEntries, applicationCacheApplicationDefinitions, applicationCachePolicies); err != nil {
		panic(err)
	}
}

func validateCategoryResolverRegistry(entries []categoryCatalogEntry) error {
	for _, entry := range entries {
		id := entry.definition.Identifier
		executable := isExecutableCategoryEligibility(entry.definition.Eligibility)
		if executable && entry.resolver == nil {
			return fmt.Errorf("executable cleanup category %q is missing a resolver", id)
		}
		if !executable && entry.resolver != nil {
			return fmt.Errorf("non-executable cleanup category %q must not register a resolver", id)
		}
		switch entry.resolverKind {
		case categoryResolverDefault:
			if entry.definition.Eligibility != CategoryEligibilityDefault {
				return fmt.Errorf("default resolver category %q must use default eligibility", id)
			}
		case categoryResolverExistenceOpportunity, categoryResolverBrowserCache,
			categoryResolverApplicationCache, categoryResolverDeveloperCache,
			categoryResolverGrokBuildUpdateResidue:
			if entry.definition.Eligibility != CategoryEligibilityOptIn {
				return fmt.Errorf("opt-in resolver category %q must use opt-in eligibility", id)
			}
			if entry.resolverKind == categoryResolverGrokBuildUpdateResidue {
				if entry.definition.RunningApplicationPolicy != RunningApplicationPolicyDistinctiveProcessIdle {
					return fmt.Errorf("grok residue category %q must use distinctive-process idle policy", id)
				}
				if entry.definition.PlannedAction != DeletionActionDeletePermanently {
					return fmt.Errorf("grok residue category %q must declare delete_permanently", id)
				}
				if len(entry.runningApplications) != 1 || entry.runningApplications[0] != ApplicationGrokBuild {
					return fmt.Errorf("grok residue category %q must bind ApplicationGrokBuild only", id)
				}
			}
		case categoryResolverNonExecutable:
			if executable {
				return fmt.Errorf("executable cleanup category %q must register a resolver kind", id)
			}
		default:
			return fmt.Errorf("cleanup category %q has unsupported resolver kind %q", id, entry.resolverKind)
		}
	}
	return nil
}

func mustCleanupCategoryCatalog(entries []categoryCatalogEntry) CleanupCategoryCatalog {
	definitions := make([]CleanupCategoryDefinition, 0, len(entries))
	for _, entry := range entries {
		definitions = append(definitions, entry.definition)
	}
	catalog, err := NewCleanupCategoryCatalog(definitions)
	if err != nil {
		panic(err)
	}
	return catalog
}

// validateDeveloperCacheRegistry rejects incomplete or ambiguous private
// developer-cache registrations. It is the construction-time consistency gate
// for the canonical registry.
func validateDeveloperCacheRegistry(
	entries []categoryCatalogEntry,
	applications []supportedApplicationDefinition,
	suggestionAllowlist map[string]reviewSuggestionTool,
) error {
	appByID := make(map[string]supportedApplicationDefinition, len(applications))
	executableOwner := make(map[string]string)
	for _, app := range applications {
		id := strings.TrimSpace(app.id)
		if id == "" {
			return fmt.Errorf("developer application definition has empty id")
		}
		if strings.TrimSpace(app.displayName) == "" {
			return fmt.Errorf("developer application %q has empty display name", id)
		}
		if len(app.executables) == 0 {
			return fmt.Errorf("developer application %q has no executable names", id)
		}
		if _, exists := appByID[id]; exists {
			return fmt.Errorf("duplicate developer application id %q", id)
		}
		appByID[id] = app
		for _, executable := range app.executables {
			exeKey := strings.ToLower(strings.TrimSpace(executable))
			if exeKey == "" {
				return fmt.Errorf("developer application %q has an empty executable name", id)
			}
			if owner, exists := executableOwner[exeKey]; exists && owner != id {
				return fmt.Errorf("ambiguous executable %q owned by both %q and %q", executable, owner, id)
			}
			executableOwner[exeKey] = id
		}
	}

	for _, entry := range entries {
		if entry.resolverKind != categoryResolverDeveloperCache {
			if entry.resolvePaths != nil {
				return fmt.Errorf("non-developer-cache category %q must not register a path resolver", entry.definition.Identifier)
			}
			if entry.discoverChildren != nil {
				return fmt.Errorf("non-developer-cache category %q must not register child candidate discovery", entry.definition.Identifier)
			}
			if len(entry.reviewSuggestionTools) > 0 {
				return fmt.Errorf("non-developer-cache category %q must not register Review suggestion tools", entry.definition.Identifier)
			}
			if entry.resolverKind == categoryResolverApplicationCache {
				// Application-cache categories are validated separately.
				continue
			}
			// Grok residue and other non-dev-cache resolvers may list running
			// applications for distinctive-process policy without being
			// developer-cache registry members.
			continue
		}
		id := entry.definition.Identifier
		if entry.resolvePaths == nil && entry.resolveRootScopes == nil {
			return fmt.Errorf("developer-cache category %q is missing a path or root-scope resolver", id)
		}
		for _, tool := range entry.reviewSuggestionTools {
			tool = strings.TrimSpace(tool)
			if tool == "" {
				return fmt.Errorf("developer-cache category %q has an empty Review suggestion tool key", id)
			}
			if _, ok := suggestionAllowlist[tool]; !ok {
				return fmt.Errorf("developer-cache category %q references unknown Review suggestion tool %q", id, tool)
			}
		}
		if entry.definition.RunningApplicationPolicy == RunningApplicationPolicyDistinctiveProcessIdle {
			if len(entry.runningApplications) == 0 {
				return fmt.Errorf("developer-cache category %q uses distinctive-process policy without applications", id)
			}
		}
		seenApps := make(map[string]bool, len(entry.runningApplications))
		for _, appID := range entry.runningApplications {
			appID = strings.TrimSpace(appID)
			if appID == "" {
				return fmt.Errorf("developer-cache category %q has an empty running application id", id)
			}
			if seenApps[appID] {
				return fmt.Errorf("developer-cache category %q has duplicate running application %q", id, appID)
			}
			seenApps[appID] = true
			if _, ok := appByID[appID]; !ok {
				return fmt.Errorf("developer-cache category %q references unknown application %q", id, appID)
			}
		}
	}
	return nil
}

func CanonicalCleanupCategoryCatalog() CleanupCategoryCatalog {
	return canonicalCleanupCategoryCatalog
}

func canonicalCategoryEntry(identifier string) (categoryCatalogEntry, bool) {
	summary, ok := canonicalCleanupCategoryCatalog.Summary(identifier)
	if !ok {
		return categoryCatalogEntry{}, false
	}
	for _, entry := range canonicalCategoryEntries {
		if entry.definition.Identifier == summary.Identifier {
			return entry, true
		}
	}
	return categoryCatalogEntry{}, false
}

func selectableCategoryIDs() []string {
	var identifiers []string
	for _, entry := range canonicalCategoryEntries {
		if entry.definition.Eligibility == CategoryEligibilityOptIn {
			identifiers = append(identifiers, entry.definition.Identifier)
		}
	}
	return identifiers
}

func opportunityCategoryIDs(includeGated bool) []string {
	var identifiers []string
	for _, entry := range canonicalCategoryEntries {
		switch entry.resolverKind {
		case categoryResolverExistenceOpportunity:
			// Always included.
		case categoryResolverBrowserCache, categoryResolverApplicationCache:
			if !includeGated {
				continue
			}
		default:
			continue
		}
		identifiers = append(identifiers, entry.definition.Identifier)
	}
	return identifiers
}

func developerCacheCategoryIDs() []string {
	var identifiers []string
	for _, entry := range canonicalCategoryEntries {
		if entry.resolverKind == categoryResolverDeveloperCache {
			identifiers = append(identifiers, entry.definition.Identifier)
		}
	}
	return identifiers
}

// developerToolsOptInCategoryIDs returns Developer tools opt-in categories for
// the `dev-caches` group: registered developer-cache categories plus idle
// Application cache opportunity categories (derived from catalog policy).
// CLI-agent product categories are intentionally excluded (updater residue is
// not a cache).
func developerToolsOptInCategoryIDs() []string {
	var identifiers []string
	for _, entry := range canonicalCategoryEntries {
		if entry.definition.Eligibility != CategoryEligibilityOptIn {
			continue
		}
		if entry.definition.ReportCategory != ReportCategoryDeveloperTools {
			continue
		}
		if entry.cliAgentProduct {
			continue
		}
		if entry.resolverKind == categoryResolverDeveloperCache || entry.resolverKind == categoryResolverApplicationCache {
			identifiers = append(identifiers, entry.definition.Identifier)
		}
	}
	return identifiers
}

// cliAgentCategoryIDs returns independently registered product-scoped CLI-agent
// categories for the `cli-agents` selection group in deterministic catalog
// order. The token owns no resolver, candidates, or deletion action.
func cliAgentCategoryIDs() []string {
	var identifiers []string
	for _, entry := range canonicalCategoryEntries {
		if entry.cliAgentProduct {
			identifiers = append(identifiers, entry.definition.Identifier)
		}
	}
	return identifiers
}

func applicationCacheCategoryIDs() []string {
	var identifiers []string
	for _, entry := range canonicalCategoryEntries {
		if entry.resolverKind == categoryResolverApplicationCache {
			identifiers = append(identifiers, entry.definition.Identifier)
		}
	}
	return identifiers
}

func isApplicationCacheCategory(category string) bool {
	entry, ok := canonicalCategoryEntry(category)
	return ok && entry.resolverKind == categoryResolverApplicationCache
}

// validateApplicationCacheRegistry rejects incomplete application-cache
// registrations and ambiguous process ownership across application-cache apps.
func validateApplicationCacheRegistry(
	entries []categoryCatalogEntry,
	applications []supportedApplicationDefinition,
	policies map[string]applicationCachePolicy,
) error {
	appByID := make(map[string]supportedApplicationDefinition, len(applications))
	executableOwner := make(map[string]string)
	for _, app := range applications {
		id := strings.TrimSpace(app.id)
		if id == "" {
			return fmt.Errorf("application-cache application definition has empty id")
		}
		if strings.TrimSpace(app.displayName) == "" {
			return fmt.Errorf("application-cache application %q has empty display name", id)
		}
		if len(app.executables) == 0 {
			return fmt.Errorf("application-cache application %q has no executable names", id)
		}
		if _, exists := appByID[id]; exists {
			return fmt.Errorf("duplicate application-cache application id %q", id)
		}
		appByID[id] = app
		for _, executable := range app.executables {
			exeKey := strings.ToLower(strings.TrimSpace(executable))
			if exeKey == "" {
				return fmt.Errorf("application-cache application %q has an empty executable name", id)
			}
			if owner, exists := executableOwner[exeKey]; exists && owner != id {
				return fmt.Errorf("ambiguous executable %q owned by both %q and %q", executable, owner, id)
			}
			executableOwner[exeKey] = id
		}
	}

	for _, entry := range entries {
		if entry.resolverKind != categoryResolverApplicationCache {
			if entry.applicationCachePolicyID != "" {
				return fmt.Errorf("non-application-cache category %q must not register an application cache policy", entry.definition.Identifier)
			}
			continue
		}
		id := entry.definition.Identifier
		if entry.definition.RunningApplicationPolicy != RunningApplicationPolicyApplicationIdleBeforeAfter {
			return fmt.Errorf("application-cache category %q must use application-idle-before-and-after-inspection policy", id)
		}
		if entry.applicationCachePolicyID == "" {
			return fmt.Errorf("application-cache category %q is missing a policy id", id)
		}
		policy, ok := policies[entry.applicationCachePolicyID]
		if !ok {
			return fmt.Errorf("application-cache category %q references unknown policy %q", id, entry.applicationCachePolicyID)
		}
		if policy.category != id {
			return fmt.Errorf("application-cache category %q policy category mismatch %q", id, policy.category)
		}
		if len(entry.runningApplications) == 0 {
			return fmt.Errorf("application-cache category %q has no running applications", id)
		}
		seenApps := make(map[string]bool, len(entry.runningApplications))
		for _, appID := range entry.runningApplications {
			appID = strings.TrimSpace(appID)
			if appID == "" {
				return fmt.Errorf("application-cache category %q has an empty running application id", id)
			}
			if seenApps[appID] {
				return fmt.Errorf("application-cache category %q has duplicate running application %q", id, appID)
			}
			seenApps[appID] = true
			if _, ok := appByID[appID]; !ok {
				return fmt.Errorf("application-cache category %q references unknown application %q", id, appID)
			}
		}
		if policy.application != "" && !seenApps[policy.application] {
			return fmt.Errorf("application-cache category %q policy application %q is not registered on the entry", id, policy.application)
		}
		if len(policy.relativeRoots) == 0 {
			return fmt.Errorf("application-cache policy %q has an empty relative-root allowlist", entry.applicationCachePolicyID)
		}
		if len(policy.roamingAppDataPath) == 0 {
			return fmt.Errorf("application-cache policy %q has an empty roaming AppData path", entry.applicationCachePolicyID)
		}
		seenRoots := make(map[string]bool, len(policy.relativeRoots))
		for _, root := range policy.relativeRoots {
			if strings.TrimSpace(root) == "" || root != strings.TrimSpace(root) {
				return fmt.Errorf("application-cache policy %q has an invalid relative root %q", entry.applicationCachePolicyID, root)
			}
			if strings.Contains(root, `\`) || strings.Contains(root, `/`) || strings.Contains(root, "..") {
				return fmt.Errorf("application-cache policy %q relative root %q must be a single path segment", entry.applicationCachePolicyID, root)
			}
			key := strings.ToLower(root)
			if seenRoots[key] {
				return fmt.Errorf("application-cache policy %q has duplicate relative root %q", entry.applicationCachePolicyID, root)
			}
			seenRoots[key] = true
		}
	}
	return nil
}

func categoryReportGroup(identifier string) ReportCategory {
	entry, ok := canonicalCategoryEntry(identifier)
	if !ok {
		return ""
	}
	return entry.definition.ReportCategory
}

func developerApplicationDefinition(id string) (supportedApplicationDefinition, bool) {
	for _, app := range developerApplicationDefinitions {
		if app.id == id {
			return app, true
		}
	}
	for _, app := range applicationCacheApplicationDefinitions {
		if app.id == id {
			return app, true
		}
	}
	return supportedApplicationDefinition{}, false
}

func applicationCacheEntry(identifier string) (categoryCatalogEntry, bool) {
	entry, ok := canonicalCategoryEntry(identifier)
	if !ok || entry.resolverKind != categoryResolverApplicationCache {
		return categoryCatalogEntry{}, false
	}
	return entry, true
}
