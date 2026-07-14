package clean

import (
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
	RunningApplicationPolicyNotApplicable          RunningApplicationPolicy = "not-applicable"
	RunningApplicationPolicyBrowserIdleBeforeAfter RunningApplicationPolicy = "browser-idle-before-and-after-inspection"
	RunningApplicationPolicyDistinctiveProcessIdle RunningApplicationPolicy = "distinctive-process-must-be-idle"
	RunningApplicationPolicySharedRuntime          RunningApplicationPolicy = "shared-runtime-not-attributable"
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
}

// CleanupCategorySummary is the stable, path-free projection intended for
// read-model consumers such as the Clean TUI.
type CleanupCategorySummary struct {
	Identifier               string                   `json:"identifier"`
	Label                    string                   `json:"label"`
	ReportCategory           ReportCategory           `json:"report_category"`
	Eligibility              CategoryEligibility      `json:"eligibility"`
	RunningApplicationPolicy RunningApplicationPolicy `json:"running_application_policy"`
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
		if definition.Identifier == "" || definition.Label == "" || definition.ReportCategory == "" ||
			definition.Eligibility == "" || definition.RunningApplicationPolicy == "" {
			return CleanupCategoryCatalog{}, fmt.Errorf("cleanup category %q has incomplete metadata", definition.Identifier)
		}
		if !validReportCategory(definition.ReportCategory) || !validCategoryEligibility(definition.Eligibility) ||
			!validRunningApplicationPolicy(definition.RunningApplicationPolicy) {
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
		RunningApplicationPolicyDistinctiveProcessIdle, RunningApplicationPolicySharedRuntime:
		return true
	default:
		return false
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
// surface (browsers are prepended separately).
var developerApplicationDefinitions = []supportedApplicationDefinition{
	{id: ApplicationGo, displayName: "Go", executables: []string{"go.exe"}},
	{id: ApplicationCargo, displayName: "Cargo", executables: []string{"cargo.exe"}},
	{id: ApplicationDotNet, displayName: ".NET", executables: []string{"dotnet.exe"}},
	{id: ApplicationNuGet, displayName: "NuGet", executables: []string{"nuget.exe"}},
	{id: ApplicationNode, displayName: "Node.js", executables: []string{"node.exe"}},
	{id: ApplicationPython, displayName: "Python", executables: []string{"python.exe"}},
}

// categoryCatalogEntry is the private canonical registration point. Public
// catalog projections expose only path-free definition fields. Developer-cache
// entries additionally bind a path resolver, optional Review suggestion tool
// keys, and the applications required by the running-application policy.
type categoryCatalogEntry struct {
	definition            CleanupCategoryDefinition
	opportunity           bool
	developerCache        bool
	fixedLocalAppDataPath []string
	runningApplications   []string
	// resolvePaths resolves env/default roots for a developer-cache category.
	// Required when developerCache is true; ignored otherwise.
	resolvePaths func(devCachePathDependencies) []string
	// reviewSuggestionTools lists Review suggestion allowlist tool keys
	// associated with this developer-cache category. Empty when no suggestion
	// probe is associated (for example cargo). Referenced keys must exist.
	reviewSuggestionTools []string
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

var canonicalCategoryEntries = []categoryCatalogEntry{
	{definition: categoryDefinition(DefaultCategoryFoalOwnedTempSandboxes, "Foal-owned temp sandboxes", ReportCategoryUserEssentials, CategoryEligibilityDefault, RunningApplicationPolicyNotApplicable)},
	{definition: categoryDefinition(OpportunityCategoryUserTemp, "User temp", ReportCategoryUserEssentials, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable), opportunity: true},
	{definition: categoryDefinition(OpportunityCategoryCrashDumps, "Crash dumps", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable), opportunity: true, fixedLocalAppDataPath: []string{"CrashDumps"}},
	{definition: categoryDefinition(OpportunityCategoryWindowsErrorReporting, "Windows Error Reporting", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable), opportunity: true, fixedLocalAppDataPath: []string{"Microsoft", "Windows", "WER"}},
	{definition: categoryDefinition(OpportunityCategoryExplorerThumbnailCache, "Explorer thumbnail cache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable), opportunity: true, fixedLocalAppDataPath: []string{"Microsoft", "Windows", "Explorer"}},
	{definition: categoryDefinition(OpportunityCategoryINetCache, "INetCache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable), opportunity: true, fixedLocalAppDataPath: []string{"Microsoft", "Windows", "INetCache"}},
	{definition: categoryDefinition(OpportunityCategoryD3DShaderCache, "D3D shader cache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable), opportunity: true, fixedLocalAppDataPath: []string{"D3DSCache"}},
	{definition: categoryDefinition(OpportunityCategoryNVIDIADXCache, "NVIDIA DX cache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable), opportunity: true, fixedLocalAppDataPath: []string{"NVIDIA", "DXCache"}},
	{definition: categoryDefinition(OpportunityCategoryBrowserCache, "Browser cache", ReportCategoryBrowsers, CategoryEligibilityOptIn, RunningApplicationPolicyBrowserIdleBeforeAfter), opportunity: true},
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryNPM, "npm cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime),
		resolveNPMCachePaths,
		[]string{"npm"},
	),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryGo, "Go build cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle),
		resolveGoCachePaths,
		[]string{"go"},
		ApplicationGo,
	),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryPip, "pip cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime),
		resolvePipCachePaths,
		[]string{"pip"},
	),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryCargo, "Cargo cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle),
		resolveCargoCachePaths,
		nil,
		ApplicationCargo,
	),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryNuGet, "NuGet cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle),
		resolveNuGetCachePaths,
		[]string{"dotnet"},
		ApplicationDotNet, ApplicationNuGet,
	),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryNuGetGlobalPackages, "NuGet global packages", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle),
		resolveNuGetGlobalPackagesPaths,
		[]string{"dotnet"},
		ApplicationDotNet, ApplicationNuGet,
	),
	developerCacheEntry(
		categoryDefinition(DevCacheCategoryCorepack, "Corepack cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime),
		resolveCorepackOptInCachePaths,
		[]string{"corepack"},
	),
	{definition: categoryDefinition("administrator_only_caches", "Administrator-only caches", ReportCategorySystem, CategoryEligibilityPermissionBoundary, RunningApplicationPolicyNotApplicable)},
}

func categoryDefinition(identifier, label string, reportCategory ReportCategory, eligibility CategoryEligibility, runningPolicy RunningApplicationPolicy) CleanupCategoryDefinition {
	return CleanupCategoryDefinition{
		Identifier:               identifier,
		Label:                    label,
		ReportCategory:           reportCategory,
		Eligibility:              eligibility,
		Aliases:                  []string{},
		RunningApplicationPolicy: runningPolicy,
	}
}

var canonicalCleanupCategoryCatalog = mustCleanupCategoryCatalog(canonicalCategoryEntries)

func init() {
	// Private developer-cache validation runs after package-level vars (including
	// the Review suggestion allowlist) are initialized.
	if err := validateDeveloperCacheRegistry(canonicalCategoryEntries, developerApplicationDefinitions, reviewSuggestionAllowlist); err != nil {
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
			if len(entry.reviewSuggestionTools) > 0 {
				return fmt.Errorf("non-developer-cache category %q must not register Review suggestion tools", entry.definition.Identifier)
			}
			continue
		}
		id := entry.definition.Identifier
		if entry.resolvePaths == nil {
			return fmt.Errorf("developer-cache category %q is missing a path resolver", id)
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

func opportunityCategoryIDs(includeBrowser bool) []string {
	var identifiers []string
	for _, entry := range canonicalCategoryEntries {
		if !entry.opportunity || (!includeBrowser && entry.definition.Identifier == OpportunityCategoryBrowserCache) {
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
	return supportedApplicationDefinition{}, false
}
