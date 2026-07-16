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
}

// applicationCacheApplicationDefinitions is the controlled registry of idle
// Application cache owners. Process detection is shared with
// DetectSupportedApplications after developer tools. Each editor is an
// independent logical application so gates never cross-authorize.
var applicationCacheApplicationDefinitions = []supportedApplicationDefinition{
	{id: ApplicationVisualStudioCode, displayName: "Visual Studio Code", executables: []string{"Code.exe"}},
	{id: ApplicationCursor, displayName: "Cursor", executables: []string{"Cursor.exe"}},
}

// categoryCatalogEntry is the private canonical registration point. Public
// catalog projections expose only path-free definition fields. Developer-cache
// entries additionally bind a path resolver, optional structured child candidate
// discovery policy, optional Review suggestion tool keys, and the applications
// required by the running-application policy. Application-cache entries bind an
// idle Application cache policy id.
//
// Path resolvers, allowlists, structural matchers, and executable paths never
// appear on public CleanupCategoryDefinition / CleanupCategorySummary.
type categoryCatalogEntry struct {
	definition            CleanupCategoryDefinition
	opportunity           bool
	developerCache        bool
	applicationCache      bool
	fixedLocalAppDataPath []string
	runningApplications   []string
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

func developerCacheEntry(
	definition CleanupCategoryDefinition,
	resolvePaths func(devCachePathDependencies) []string,
	reviewSuggestionTools []string,
	runningApplications ...string,
) categoryCatalogEntry {
	return categoryCatalogEntry{
		definition:            definition,
		developerCache:        true,
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
	// 18 delete_permanently + 6 move_to_recycle_bin + 1 actionless permission boundary.
	// Over-broad whole-root system caches stay Recycle Bin until exact allowlists exist.
	{definition: categoryDefinition(DefaultCategoryFoalOwnedTempSandboxes, "Foal-owned temp sandboxes", ReportCategoryUserEssentials, CategoryEligibilityDefault, RunningApplicationPolicyNotApplicable, DeletionActionMoveToRecycleBin)},
	{definition: categoryDefinition(OpportunityCategoryUserTemp, "User temp", ReportCategoryUserEssentials, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionMoveToRecycleBin), opportunity: true},
	{definition: categoryDefinition(OpportunityCategoryCrashDumps, "Crash dumps", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionMoveToRecycleBin), opportunity: true, fixedLocalAppDataPath: []string{"CrashDumps"}},
	{definition: categoryDefinition(OpportunityCategoryWindowsErrorReporting, "Windows Error Reporting", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionMoveToRecycleBin), opportunity: true, fixedLocalAppDataPath: []string{"Microsoft", "Windows", "WER"}},
	{definition: categoryDefinition(OpportunityCategoryExplorerThumbnailCache, "Explorer thumbnail cache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionMoveToRecycleBin), opportunity: true, fixedLocalAppDataPath: []string{"Microsoft", "Windows", "Explorer"}},
	{definition: categoryDefinition(OpportunityCategoryINetCache, "INetCache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionMoveToRecycleBin), opportunity: true, fixedLocalAppDataPath: []string{"Microsoft", "Windows", "INetCache"}},
	{definition: categoryDefinition(OpportunityCategoryD3DShaderCache, "D3D shader cache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionDeletePermanently), opportunity: true, fixedLocalAppDataPath: []string{"D3DSCache"}},
	{definition: categoryDefinition(OpportunityCategoryNVIDIADXCache, "NVIDIA DX cache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, DeletionActionDeletePermanently), opportunity: true, fixedLocalAppDataPath: []string{"NVIDIA", "DXCache"}},
	{definition: categoryDefinition(OpportunityCategoryBrowserCache, "Browser cache", ReportCategoryBrowsers, CategoryEligibilityOptIn, RunningApplicationPolicyBrowserIdleBeforeAfter, DeletionActionDeletePermanently), opportunity: true},
	{
		definition: categoryDefinition(
			OpportunityCategoryVSCodeCache,
			"VS Code cache",
			ReportCategoryDeveloperTools,
			CategoryEligibilityOptIn,
			RunningApplicationPolicyApplicationIdleBeforeAfter,
			DeletionActionDeletePermanently,
		),
		opportunity:              true,
		applicationCache:         true,
		applicationCachePolicyID: applicationCachePolicyVSCode,
		runningApplications:      []string{ApplicationVisualStudioCode},
	},
	{
		definition: categoryDefinition(
			OpportunityCategoryCursorCache,
			"Cursor cache",
			ReportCategoryDeveloperTools,
			CategoryEligibilityOptIn,
			RunningApplicationPolicyApplicationIdleBeforeAfter,
			DeletionActionDeletePermanently,
		),
		opportunity:              true,
		applicationCache:         true,
		applicationCachePolicyID: applicationCachePolicyCursor,
		runningApplications:      []string{ApplicationCursor},
	},
	// Package and build caches: proven regenerable roots; env/default
	// resolvers, gates, and impact notices unchanged.
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryNPM, "npm cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime, DeletionActionDeletePermanently),
		resolveNPMCachePaths,
		[]string{"npm"},
	),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryGo, "Go build cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle, DeletionActionDeletePermanently),
		resolveGoCachePaths,
		[]string{"go"},
		ApplicationGo,
	),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryPip, "pip cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime, DeletionActionDeletePermanently),
		resolvePipCachePaths,
		[]string{"pip"},
	),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryCargo, "Cargo cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle, DeletionActionDeletePermanently),
		resolveCargoCachePaths,
		nil,
		ApplicationCargo,
	),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryNuGet, "NuGet cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle, DeletionActionDeletePermanently),
		resolveNuGetCachePaths,
		[]string{"dotnet"},
		ApplicationDotNet, ApplicationNuGet,
	),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryNuGetGlobalPackages, "NuGet global packages", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle, DeletionActionDeletePermanently),
		resolveNuGetGlobalPackagesPaths,
		[]string{"dotnet"},
		ApplicationDotNet, ApplicationNuGet,
	),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryCorepack, "Corepack cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime, DeletionActionDeletePermanently),
		resolveCorepackOptInCachePaths,
		[]string{"corepack"},
	),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryUV, "uv cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle, DeletionActionDeletePermanently),
		resolveUVCachePaths,
		[]string{"uv"},
		ApplicationUV,
	),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryBun, "Bun cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle, DeletionActionDeletePermanently),
		resolveBunCachePaths,
		[]string{"bun"},
		ApplicationBun,
	),
	// Playwright global browser binaries: structured child discovery under the
	// env/default root. Shared-runtime policy: do not attribute node/chrome/etc.
	// as Playwright-owned. Permanent deletion; no Review suggestion probe.
	developerCacheEntryWithChildren(
		categoryDefinition(DevCacheCategoryPlaywright, "Playwright browsers", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime, DeletionActionDeletePermanently),
		resolvePlaywrightBrowserPaths,
		discoverPlaywrightBrowserChildren,
		nil,
	),
	// Puppeteer browser binaries: shared-runtime (Node/Python/Chrome/Firefox are
	// not attributable). Structured children under env/default root only; root
	// and product parents are never candidates (ADR 0011). Permanent deletion.
	developerCacheEntryWithChildren(
		categoryDefinition(DevCacheCategoryPuppeteerBrowsers, "Puppeteer browsers", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime, DeletionActionDeletePermanently),
		resolvePuppeteerCachePaths,
		discoverPuppeteerBrowserChildren,
		nil,
	),
	// Electron download cache: whole-root under env/default only. Shared-runtime
	// (Node/Electron hosts are not attributable). Permanent deletion; no
	// Review suggestion probe and no invented Electron cleanup command.
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryElectron, "Electron cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime, DeletionActionDeletePermanently),
		resolveElectronCachePaths,
		nil,
	),
	// JetBrains IDE system caches: product-scoped roots under %LOCALAPPDATA%\JetBrains
	// with structured children (caches/index; Rider also resharper-host). Distinctive-
	// process policy; each product identity gates independently via
	// DevCacheRootScope.Application. Permanent deletion; no Review probe.
	developerCacheEntryWithProductScopedChildren(
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
	),
	// Non-executable permission boundary: actionless and cannot enter execution.
	{definition: categoryDefinition("administrator_only_caches", "Administrator-only caches", ReportCategorySystem, CategoryEligibilityPermissionBoundary, RunningApplicationPolicyNotApplicable, "")},
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
	// Private developer-cache validation runs after package-level vars (including
	// the Review suggestion allowlist) are initialized.
	if err := validateDeveloperCacheRegistry(canonicalCategoryEntries, developerApplicationDefinitions, reviewSuggestionAllowlist); err != nil {
		panic(err)
	}
	if err := validateApplicationCacheRegistry(canonicalCategoryEntries, applicationCacheApplicationDefinitions, applicationCachePolicies); err != nil {
		panic(err)
	}
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
		if !entry.developerCache {
			if entry.resolvePaths != nil {
				return fmt.Errorf("non-developer-cache category %q must not register a path resolver", entry.definition.Identifier)
			}
			if entry.discoverChildren != nil {
				return fmt.Errorf("non-developer-cache category %q must not register child candidate discovery", entry.definition.Identifier)
			}
			if len(entry.reviewSuggestionTools) > 0 {
				return fmt.Errorf("non-developer-cache category %q must not register Review suggestion tools", entry.definition.Identifier)
			}
			if entry.applicationCache {
				// Application-cache categories are validated separately.
				continue
			}
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
		if !entry.opportunity {
			continue
		}
		// Browser and idle Application cache categories use dedicated gated
		// discovery; the generic existence/user-temp scanner must omit them.
		if !includeGated && (entry.definition.Identifier == OpportunityCategoryBrowserCache || entry.applicationCache) {
			continue
		}
		identifiers = append(identifiers, entry.definition.Identifier)
	}
	return identifiers
}

func developerCacheCategoryIDs() []string {
	var identifiers []string
	for _, entry := range canonicalCategoryEntries {
		if entry.developerCache {
			identifiers = append(identifiers, entry.definition.Identifier)
		}
	}
	return identifiers
}

// developerToolsOptInCategoryIDs returns Developer tools opt-in categories for
// the `dev-caches` group: registered developer-cache categories plus idle
// Application cache opportunity categories (derived from catalog policy).
func developerToolsOptInCategoryIDs() []string {
	var identifiers []string
	for _, entry := range canonicalCategoryEntries {
		if entry.definition.Eligibility != CategoryEligibilityOptIn {
			continue
		}
		if entry.definition.ReportCategory != ReportCategoryDeveloperTools {
			continue
		}
		if entry.developerCache || entry.applicationCache {
			identifiers = append(identifiers, entry.definition.Identifier)
		}
	}
	return identifiers
}

func applicationCacheCategoryIDs() []string {
	var identifiers []string
	for _, entry := range canonicalCategoryEntries {
		if entry.applicationCache {
			identifiers = append(identifiers, entry.definition.Identifier)
		}
	}
	return identifiers
}

func isApplicationCacheCategory(category string) bool {
	entry, ok := canonicalCategoryEntry(category)
	return ok && entry.applicationCache
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
		if !entry.applicationCache {
			if entry.applicationCachePolicyID != "" {
				return fmt.Errorf("non-application-cache category %q must not register an application cache policy", entry.definition.Identifier)
			}
			continue
		}
		id := entry.definition.Identifier
		if !entry.opportunity {
			return fmt.Errorf("application-cache category %q must be an opportunity", id)
		}
		if entry.developerCache {
			return fmt.Errorf("application-cache category %q must not also be a developer-cache", id)
		}
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
	if !ok || !entry.applicationCache {
		return categoryCatalogEntry{}, false
	}
	return entry, true
}
