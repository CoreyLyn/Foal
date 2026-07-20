package clean

import (
	"fmt"
	"strings"
	"time"
)

type PreviewReadModel struct {
	Title                            string
	Status                           string
	ProtectionRules                  []PreviewProtectionRule
	ProtectionDiagnostics            []ProtectionDiagnostic
	Candidates                       []PreviewCandidate
	Skipped                          []PreviewSkippedItem
	SkippedByDefault                 []PreviewSkippedByDefaultItem
	Opportunities                    []Opportunity
	IncompleteOpportunityInspections []IncompleteOpportunityInspection
	ReviewClues                      []PreviewReviewClue
	ReviewSuggestions                []PreviewReviewSuggestion
	RunningApplicationSkips          []PreviewRunningApplicationSkip
	Errors                           []StructuredIssue
	Notices                          []PreviewNotice
	PotentialSpaceBytes              int64
	CandidateCount                   int
	SkippedCount                     int
	OpportunityCount                 int
	OpportunityObservedBytes         int64
	OptInReclaimableBytes            int64
	OptInCategories                  []PreviewOptInCategory
	DetailedListPath                 string
	Summary                          string
}

// PreviewOptInCategory is the path-free selection projection consumed by the
// Clean TUI. Candidate paths remain in the existing preview collections and
// are never part of selection state.
type PreviewOptInCategory struct {
	Identifier     string
	Label          string
	ReportCategory ReportCategory
	// PlannedAction is the catalog-owned deletion action for this opt-in category.
	PlannedAction    PlannedAction
	Selected         bool
	CandidateCount   int
	ReclaimableBytes int64
	ObservedBytes    int64
}

type PreviewProtectionRule struct {
	ID          string
	Description string
	Path        string
	UserDefined bool
}

type PreviewCandidate struct {
	Path          string
	Bytes         int64
	Rule          string
	PlannedAction string
}

type PreviewSkippedItem struct {
	Path   string
	Bytes  int64
	Rule   string
	Reason StructuredIssue
}

type PreviewSkippedByDefaultItem struct {
	Name   string
	Path   string
	Bytes  int64
	Reason string
}

type PreviewReviewClue struct {
	Name    string
	Path    string
	Details string
}

type PreviewReviewSuggestion struct {
	Tool      string
	Label     string
	Command   string
	CachePath string
	NextStep  string
}

type PreviewRunningApplicationSkip struct {
	Name        string
	Path        string
	Application string
	Reason      string
}

type PreviewNotice struct {
	Kind    string
	Message string
}

type previewReportPresentation struct {
	defaultCandidateLabel string
	skippedLabel          string
	inspectionErrorLabel  string
}

var plainPreviewReportPresentation = previewReportPresentation{
	defaultCandidateLabel: "default candidate",
	skippedLabel:          "skipped",
	inspectionErrorLabel:  "inspection error",
}

const previewReportSectionEntryLimit = 10

const ReviewSuggestionSafetyNote = "Clearing a tool cache while the tool is installing or building can disrupt that operation. Confirm the tool is idle first."
const administratorOnlyCacheBoundaryNotice = "Permission boundary: administrator-only caches such as SoftwareDistribution and Delivery Optimization are excluded from Opportunity discovery. Foal will not request elevation automatically."

// uvCacheOptInImpactNotice is shown when uv-cache is an Opt-in candidate. uv
// rebuilds disposable tool environments and re-downloads dependencies after a
// cache reclaim; Foal must not present this as zero-impact cleanup. Upstream
// also advises against modifying the cache directory directly.
const uvCacheOptInImpactNotice = "Opt-in uv cache cleanup may require re-downloading dependencies and rebuilding disposable tool environments. It is not zero-impact."

// nugetGlobalPackagesOptInImpactNotice is a high-impact warning for
// nuget-global-packages. Packages may restore on the next build, but offline,
// private-source, removed, or inaccessible packages may not be recoverable.
const nugetGlobalPackagesOptInImpactNotice = "Opt-in NuGet global packages cleanup will require builds to restore packages again. Offline, private-source, removed, or otherwise inaccessible packages may not be recoverable."

// bunCacheOptInImpactNotice is shown when bun-cache is an Opt-in candidate.
// Clearing Bun's global install cache may require future dependency downloads,
// and hardlinked project content can reduce actual reclaimable disk space.
const bunCacheOptInImpactNotice = "Opt-in Bun cache cleanup may require re-downloading dependencies. Hardlinked project content can affect actual disk space reclaimed."

// pnpmCacheOptInImpactNotice is shown when pnpm-cache is an Opt-in candidate.
// Clearing pnpm's content-addressable store requires re-downloading packages;
// hardlinked project content and offline workflows may be affected.
const pnpmCacheOptInImpactNotice = "Opt-in pnpm store cleanup may require re-downloading packages. Hardlinked project content and offline installs can be affected."

// yarnCacheOptInImpactNotice is shown when yarn-cache is an Opt-in candidate.
// Clearing Yarn's global package cache may require future dependency downloads
// and can slow the next install offline.
const yarnCacheOptInImpactNotice = "Opt-in yarn cache cleanup may require re-downloading dependencies. Offline installs and private-registry packages may be affected."

// goModCacheOptInImpactNotice is shown when go-modcache is an Opt-in candidate.
// Modules must be re-downloaded; offline / private module sources may fail to restore.
const goModCacheOptInImpactNotice = "Opt-in Go module cache cleanup requires re-downloading modules. Offline workflows and private or removed module sources may fail to restore."

// cargoCacheOptInImpactNotice is shown when cargo-cache is an Opt-in candidate.
// Allowlisted registry/cache (.crate archives) and registry/src (unpacked crate
// sources) re-download/re-extract on the next build; offline and private-registry
// workflows may be affected.
const cargoCacheOptInImpactNotice = "Opt-in Cargo cache cleanup may require re-downloading crate archives and re-extracting crate sources. Offline builds and private-registry packages may be affected."

// playwrightBrowsersOptInImpactNotice is shown when playwright-browsers has
// Opt-in candidates. Installations are re-downloadable but may be large, needed
// offline, or in use by active automation; Foal does not stop processes.
const playwrightBrowsersOptInImpactNotice = "Opt-in Playwright browser cleanup reclaims re-downloadable browser installations. Offline workflows may need those browsers again, and active automation may fail if an installation is in use."

// puppeteerBrowsersOptInImpactNotice is shown when puppeteer-browsers has Opt-in
// candidates. Installations are re-downloadable; offline automation workflows
// may need them again, and active automation can cause cleanup failure. Foal
// does not stop Node, Python, Chrome, or Firefox processes.
const puppeteerBrowsersOptInImpactNotice = "Opt-in Puppeteer browser cleanup reclaims re-downloadable browser installations. Offline workflows may need those browsers again, and active automation can cause cleanup failure. Foal does not stop Node, Python, Chrome, or Firefox processes."

// electronCacheOptInImpactNotice is shown when electron-cache is an Opt-in
// candidate. Cached Electron binaries may need to be downloaded again; offline
// and custom-cache workflows may be affected. Foal does not stop Electron/Node.
const electronCacheOptInImpactNotice = "Opt-in Electron cache cleanup may require re-downloading cached Electron binaries. Offline and custom-cache workflows may be affected."

// jetbrainsIDECachesOptInImpactNotice is defined in jetbrains_ide_caches.go and
// shown when jetbrains-ide-caches has Opt-in candidates. Indexes rebuild; the
// next IDE startup or project open may be slower.

// visualStudioCachesOptInImpactNotice is defined in visual_studio_caches.go and
// shown when visual-studio-caches has Opt-in candidates. MEF/Roslyn caches
// rebuild; the next Visual Studio startup or solution load may be slower.

// applicationCacheCachedExtensionVSIXsImpactNotice is shown when an Application
// cache CachedExtensionVSIXs root is observed or selected. Installed extensions
// and settings are never selected for any editor category.
const applicationCacheCachedExtensionVSIXsImpactNotice = "CachedExtensionVSIXs holds downloaded extension packages that may need to be fetched again after cleanup. Installed extensions and settings are not selected."

type PreviewReportCategory struct {
	Name  string
	Lines []string
}

type PreviewReportCategoryOptions struct {
	EntryLimit                   int
	Expanded                     bool
	Compact                      bool
	ByteFormatter                func(int64) string
	IncludeCandidates            bool
	IncludeSkipped               bool
	IncludeReview                bool
	IncludeErrors                bool
	IncludeIncompleteInspections bool
	IncludeProtectionDiagnostics bool
	IncludeSummary               bool
	PreviewSummary               bool
}

func NewPreviewReadModel(result Result) PreviewReadModel {
	return NewPreviewReadModelForSelection(result, nil)
}

func NewPreviewReadModelForSelection(result Result, selected []string) PreviewReadModel {
	candidates := make([]PreviewCandidate, 0, len(result.Candidates))
	var potentialSpace int64
	for _, candidate := range result.Candidates {
		potentialSpace += candidate.Bytes
		candidates = append(candidates, PreviewCandidate{
			Path:          candidate.Path,
			Bytes:         candidate.Bytes,
			Rule:          candidate.Rule,
			PlannedAction: candidate.PlannedAction,
		})
	}

	protectionRules := make([]PreviewProtectionRule, 0, len(result.DefaultRuleCatalog)+len(result.ProtectionRules))
	for _, rule := range result.DefaultRuleCatalog {
		if !rule.DefaultEnabled {
			continue
		}
		protectionRules = append(protectionRules, PreviewProtectionRule{
			ID:          rule.ID,
			Description: rule.Description,
		})
	}
	for _, rule := range result.ProtectionRules {
		protectionRules = append(protectionRules, PreviewProtectionRule{
			Path:        rule.Path,
			UserDefined: true,
		})
	}

	skippedItems := make([]PreviewSkippedItem, 0, len(result.Skipped))
	hasPermissionBoundary := false
	for _, skipped := range result.Skipped {
		if isPermissionBoundaryCode(skipped.Reason.Code) {
			hasPermissionBoundary = true
		}
		skippedItems = append(skippedItems, PreviewSkippedItem{
			Path:   skipped.Path,
			Bytes:  skipped.Bytes,
			Rule:   skipped.Rule,
			Reason: skipped.Reason,
		})
	}
	for _, err := range result.Errors {
		if isPermissionBoundaryCode(err.Code) {
			hasPermissionBoundary = true
		}
	}

	notices := []PreviewNotice{{
		Kind:    "permission_boundary",
		Message: administratorOnlyCacheBoundaryNotice,
	}}
	if hasPermissionBoundary {
		notices = append(notices, PreviewNotice{
			Kind:    "permission_boundary",
			Message: "Permission boundary: Foal skipped protected or administrator-only locations during preview. Review the skipped entries as boundaries; Foal will not request elevation automatically.",
		})
	}
	var hasBrowserCacheOptIn, hasUVOptIn, hasNuGetGlobalPackagesOptIn, hasBunOptIn, hasPNPMOptIn, hasYarnOptIn, hasGoModCacheOptIn, hasCargoOptIn, hasPlaywrightOptIn, hasPuppeteerOptIn, hasElectronOptIn, hasJetBrainsOptIn, hasVisualStudioOptIn, hasApplicationCacheVSIX bool
	for _, candidate := range result.OptInCandidates {
		switch candidate.Category {
		case OpportunityCategoryBrowserCache:
			hasBrowserCacheOptIn = true
		case DevCacheCategoryUV:
			hasUVOptIn = true
		case DevCacheCategoryNuGetGlobalPackages:
			hasNuGetGlobalPackagesOptIn = true
		case DevCacheCategoryBun:
			hasBunOptIn = true
		case DevCacheCategoryPNPM:
			hasPNPMOptIn = true
		case DevCacheCategoryYarn:
			hasYarnOptIn = true
		case DevCacheCategoryGoModCache:
			hasGoModCacheOptIn = true
		case DevCacheCategoryCargo:
			hasCargoOptIn = true
		case DevCacheCategoryPlaywright:
			hasPlaywrightOptIn = true
		case DevCacheCategoryPuppeteerBrowsers:
			hasPuppeteerOptIn = true
		case DevCacheCategoryElectron:
			hasElectronOptIn = true
		case DevCacheCategoryJetBrainsIDECaches:
			hasJetBrainsOptIn = true
		case DevCacheCategoryVisualStudioCaches:
			hasVisualStudioOptIn = true
		default:
			if isApplicationCacheCategory(candidate.Category) && isCachedExtensionVSIXsPath(candidate.Path) {
				hasApplicationCacheVSIX = true
			}
		}
	}
	for _, opportunity := range result.Opportunities {
		category := normalizedOpportunityCategory(opportunity.Category)
		if isApplicationCacheCategory(category) && isCachedExtensionVSIXsPath(opportunity.Path) {
			hasApplicationCacheVSIX = true
		}
	}
	if hasBrowserCacheOptIn {
		notices = append(notices, PreviewNotice{
			Kind:    "opt_in_impact",
			Message: browserCacheOptInImpactNotice,
		})
	}
	if hasUVOptIn {
		notices = append(notices, PreviewNotice{
			Kind:    "opt_in_impact",
			Message: uvCacheOptInImpactNotice,
		})
	}
	if hasNuGetGlobalPackagesOptIn {
		notices = append(notices, PreviewNotice{
			Kind:    "opt_in_impact",
			Message: nugetGlobalPackagesOptInImpactNotice,
		})
	}
	if hasBunOptIn {
		notices = append(notices, PreviewNotice{
			Kind:    "opt_in_impact",
			Message: bunCacheOptInImpactNotice,
		})
	}
	if hasPNPMOptIn {
		notices = append(notices, PreviewNotice{
			Kind:    "opt_in_impact",
			Message: pnpmCacheOptInImpactNotice,
		})
	}
	if hasYarnOptIn {
		notices = append(notices, PreviewNotice{
			Kind:    "opt_in_impact",
			Message: yarnCacheOptInImpactNotice,
		})
	}
	if hasGoModCacheOptIn {
		notices = append(notices, PreviewNotice{
			Kind:    "opt_in_impact",
			Message: goModCacheOptInImpactNotice,
		})
	}
	if hasCargoOptIn {
		notices = append(notices, PreviewNotice{
			Kind:    "opt_in_impact",
			Message: cargoCacheOptInImpactNotice,
		})
	}
	if hasPlaywrightOptIn {
		notices = append(notices, PreviewNotice{
			Kind:    "opt_in_impact",
			Message: playwrightBrowsersOptInImpactNotice,
		})
	}
	if hasPuppeteerOptIn {
		notices = append(notices, PreviewNotice{
			Kind:    "opt_in_impact",
			Message: puppeteerBrowsersOptInImpactNotice,
		})
	}
	if hasElectronOptIn {
		notices = append(notices, PreviewNotice{
			Kind:    "opt_in_impact",
			Message: electronCacheOptInImpactNotice,
		})
	}
	if hasJetBrainsOptIn {
		notices = append(notices, PreviewNotice{
			Kind:    "opt_in_impact",
			Message: jetbrainsIDECachesOptInImpactNotice,
		})
	}
	if hasVisualStudioOptIn {
		notices = append(notices, PreviewNotice{
			Kind:    "opt_in_impact",
			Message: visualStudioCachesOptInImpactNotice,
		})
	}
	if hasApplicationCacheVSIX {
		notices = append(notices, PreviewNotice{
			Kind:    "opt_in_impact",
			Message: applicationCacheCachedExtensionVSIXsImpactNotice,
		})
	}

	reviewSuggestions := make([]PreviewReviewSuggestion, 0, len(result.ReviewSuggestions))
	for _, suggestion := range result.ReviewSuggestions {
		reviewSuggestions = append(reviewSuggestions, PreviewReviewSuggestion{
			Tool:      suggestion.Tool,
			Label:     suggestion.Label,
			Command:   suggestion.Command,
			CachePath: suggestion.CachePath,
		})
	}
	runningApplicationSkips := make([]PreviewRunningApplicationSkip, 0, len(result.RunningApplications))
	for _, state := range result.RunningApplications {
		if state.State != RunningApplicationStateRunning {
			continue
		}
		displayName := applicationDisplayName(state.Application)
		nameKind := "cache review"
		reasonKind := "cache review"
		if isBrowserApplication(state.Application) {
			nameKind = "browser review"
			reasonKind = "browser cache review"
		}
		runningApplicationSkips = append(runningApplicationSkips, PreviewRunningApplicationSkip{
			Name:        displayName + " " + nameKind,
			Application: displayName,
			Reason:      displayName + " is running; " + reasonKind + " was skipped.",
		})
	}
	opportunities := append([]Opportunity(nil), result.Opportunities...)
	for index := range opportunities {
		opportunities[index].Category = normalizedOpportunityCategory(opportunities[index].Category)
	}
	reviewClues := []PreviewReviewClue{{
		Name:    "Rebuildable project artifacts",
		Details: "Clean does not scan project trees. Use foal analyze <path> to label top-level rebuildable directories, or foal purge <root> for explicit-root preview and permanent reclaim.",
	}}
	selectedSet, _, _ := NormalizedOptInSet(selected)
	categorySummaries := previewOptInCategories(result, selectedSet)

	return PreviewReadModel{
		Title:                            "Foal clean",
		Status:                           "preview_only",
		ProtectionRules:                  protectionRules,
		ProtectionDiagnostics:            append([]ProtectionDiagnostic(nil), result.ProtectionDiagnostics...),
		Candidates:                       candidates,
		Skipped:                          skippedItems,
		Opportunities:                    opportunities,
		IncompleteOpportunityInspections: append([]IncompleteOpportunityInspection(nil), result.IncompleteOpportunityInspections...),
		ReviewClues:                      reviewClues,
		ReviewSuggestions:                reviewSuggestions,
		RunningApplicationSkips:          runningApplicationSkips,
		Errors:                           append([]StructuredIssue(nil), result.Errors...),
		Notices:                          notices,
		PotentialSpaceBytes:              potentialSpace,
		CandidateCount:                   len(candidates),
		SkippedCount:                     len(result.Skipped),
		OpportunityCount:                 result.Totals.OpportunityCount,
		OpportunityObservedBytes:         result.Totals.OpportunityObservedBytes,
		OptInReclaimableBytes:            result.Totals.OptInReclaimableBytes,
		OptInCategories:                  categorySummaries,
		DetailedListPath:                 result.DetailedListPath,
		Summary:                          "Dry-run summary: No changes were made. Re-run with foal clean --execute to move these default candidates to the Recycle Bin.",
	}
}

func previewOptInCategories(result Result, selected map[string]bool) []PreviewOptInCategory {
	summaries := make([]PreviewOptInCategory, 0)
	for _, group := range []ReportCategory{ReportCategorySystem, ReportCategoryUserEssentials, ReportCategoryBrowsers, ReportCategoryDeveloperTools, ReportCategoryApplications} {
		for _, category := range canonicalCleanupCategoryCatalog.Summaries() {
			if category.Eligibility != CategoryEligibilityOptIn || category.ReportCategory != group {
				continue
			}
			summary := PreviewOptInCategory{
				Identifier:     category.Identifier,
				Label:          category.Label,
				ReportCategory: category.ReportCategory,
				PlannedAction:  category.PlannedAction,
				Selected:       selected[category.Identifier],
			}
			for _, candidate := range result.OptInCandidates {
				if candidate.Category == category.Identifier {
					summary.CandidateCount++
					summary.ReclaimableBytes += candidate.Bytes
				}
			}
			for _, opportunity := range result.Opportunities {
				if normalizedOpportunityCategory(opportunity.Category) == category.Identifier {
					summary.ObservedBytes += opportunity.Bytes
				}
			}
			summaries = append(summaries, summary)
		}
	}
	return summaries
}

func applicationDisplayName(application string) string {
	switch application {
	case ApplicationGoogleChrome:
		return "Google Chrome"
	case ApplicationMicrosoftEdge:
		return "Microsoft Edge"
	case ApplicationMozillaFirefox:
		return "Mozilla Firefox"
	default:
		if app, ok := developerApplicationDefinition(application); ok {
			return app.displayName
		}
		return application
	}
}

func isPermissionBoundaryCode(code string) bool {
	return code == "protected_path" || code == "permission_denied"
}

func RenderPreviewReport(model PreviewReadModel) string {
	return renderPreviewReport(model, plainPreviewReportPresentation)
}

func RenderFailureReport(result Result) string {
	var builder strings.Builder
	builder.WriteString("Foal clean\n")
	builder.WriteString("Clean stopped before candidate scanning because required configuration could not be loaded.\n")
	if len(result.ProtectionDiagnostics) > 0 {
		builder.WriteString("\nProtection diagnostics\n")
		for _, diagnostic := range result.ProtectionDiagnostics {
			builder.WriteString(fmt.Sprintf("  %s (source: %s", diagnostic.Code, diagnostic.Source))
			if diagnostic.Line > 0 {
				builder.WriteString(fmt.Sprintf(", line %d", diagnostic.Line))
			}
			builder.WriteString(fmt.Sprintf(", recoverable: %t)\n", diagnostic.Recoverable))
		}
	}
	builder.WriteString("\nConfiguration errors\n")
	for _, issue := range result.Errors {
		builder.WriteString(fmt.Sprintf("  %s (path: %s, recoverable: %t)\n", issue.Code, issue.Path, issue.Recoverable))
		if issue.Message != "" {
			builder.WriteString(fmt.Sprintf("    %s\n", issue.Message))
		}
	}
	return builder.String()
}

func renderPreviewReport(model PreviewReadModel, presentation previewReportPresentation) string {
	var builder strings.Builder
	builder.WriteString(model.Title)
	builder.WriteString("\n")
	builder.WriteString("Preview only: Foal inspected default cleanup candidates and did not change files.\n")
	writePreviewReportCategories(&builder, previewReportCategories(model, PreviewReportCategoryOptions{
		EntryLimit:                   previewReportSectionEntryLimit,
		Expanded:                     true,
		IncludeCandidates:            true,
		IncludeSkipped:               true,
		IncludeReview:                true,
		IncludeErrors:                true,
		IncludeIncompleteInspections: true,
		IncludeProtectionDiagnostics: true,
		IncludeSummary:               true,
	}, presentation))
	return builder.String()
}

func PreviewReportCategories(model PreviewReadModel, opts PreviewReportCategoryOptions) []PreviewReportCategory {
	return previewReportCategories(model, opts, plainPreviewReportPresentation)
}

func previewReportCategories(model PreviewReadModel, opts PreviewReportCategoryOptions, presentation previewReportPresentation) []PreviewReportCategory {
	if opts.EntryLimit <= 0 {
		opts.EntryLimit = previewReportSectionEntryLimit
	}
	categories := make([]PreviewReportCategory, 0, 7)
	add := func(name string, lines []string) {
		if len(lines) == 0 {
			return
		}
		categories = append(categories, PreviewReportCategory{Name: name, Lines: lines})
	}
	if opts.IncludeReview || opts.IncludeIncompleteInspections {
		add("System", systemReportLines(model, opts))
	}
	if opts.IncludeCandidates || opts.IncludeSkipped || opts.IncludeReview || opts.IncludeIncompleteInspections {
		add("User essentials", userEssentialsReportLines(model, opts, presentation))
	}
	if opts.IncludeReview || opts.IncludeErrors || opts.IncludeIncompleteInspections {
		add("Browsers", browserReportLines(model, opts))
	}
	if opts.IncludeReview {
		add("Developer tools", developerToolReportLines(model, opts))
		add("Applications", applicationReportLines(model, opts))
		add("Project artifacts", projectArtifactReportLines(model, opts))
	}
	if opts.IncludeReview || opts.IncludeProtectionDiagnostics {
		add("Protection", protectionReportLines(model, opts))
	}
	if opts.IncludeSummary {
		add("Summary", summaryReportLines(model, opts, presentation))
	}
	return categories
}

func writePreviewReportCategories(builder *strings.Builder, categories []PreviewReportCategory) {
	for _, category := range categories {
		builder.WriteString("\n")
		builder.WriteString(category.Name)
		builder.WriteString("\n")
		for _, line := range category.Lines {
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
}

func systemReportLines(model PreviewReadModel, opts PreviewReportCategoryOptions) []string {
	var lines []string
	if len(model.Notices) > 0 {
		lines = append(lines, "  Notices")
		for _, notice := range model.Notices {
			lines = append(lines, fmt.Sprintf("    %s%s", markerPrefix(opts, "boundary"), notice.Message))
		}
	}
	systemOpportunities, omitted := categorizedOpportunities(model.Opportunities, opts.EntryLimit, func(opportunity Opportunity) bool {
		return categoryReportGroup(normalizedOpportunityCategory(opportunity.Category)) == ReportCategorySystem
	})
	if len(systemOpportunities) > 0 {
		lines = append(lines, fmt.Sprintf("  Skipped by default: %d system opportunity item(s)", len(systemOpportunities)+omitted))
		lines = append(lines, opportunityLines(systemOpportunities, opts)...)
		if omitted > 0 {
			lines = append(lines, omittedLine(omitted, model.DetailedListPath))
		}
	}
	if opts.IncludeIncompleteInspections {
		for _, incomplete := range model.IncompleteOpportunityInspections {
			if categoryReportGroup(normalizedOpportunityCategory(incomplete.Category)) != ReportCategorySystem {
				continue
			}
			lines = append(lines, incompleteInspectionLine(incomplete, opts))
		}
	}
	return lines
}

func userEssentialsReportLines(model PreviewReadModel, opts PreviewReportCategoryOptions, presentation previewReportPresentation) []string {
	var lines []string
	if opts.IncludeCandidates {
		lines = append(lines, fmt.Sprintf("  Default candidates (%d)", len(model.Candidates)))
		if len(model.Candidates) == 0 {
			lines = append(lines, fmt.Sprintf("    %sNo default candidates found.", markerPrefix(opts, "loaded")))
		} else {
			entryCount := cappedEntryCountFor(len(model.Candidates), opts.EntryLimit)
			for _, candidate := range model.Candidates[:entryCount] {
				actionLabel := plannedActionLabel(candidate.PlannedAction)
				if opts.Compact {
					label := compactPathLabel(candidate.Path)
					if opts.Expanded {
						label = candidate.Path
					}
					line := fmt.Sprintf("    [candidate] %s (%s)", label, reportFormatBytes(opts, candidate.Bytes))
					if opts.Expanded {
						line += fmt.Sprintf(" (status: %s, rule: %s, planned action: %s)", presentation.defaultCandidateLabel, candidate.Rule, actionLabel)
					}
					lines = append(lines, line)
					continue
				}
				lines = append(lines, fmt.Sprintf("    %s (%s, %srule: %s, planned action: %s)",
					candidate.Path, reportFormatBytes(opts, candidate.Bytes), statusLabel(presentation.defaultCandidateLabel), candidate.Rule, actionLabel))
			}
			if omitted := len(model.Candidates) - entryCount; omitted > 0 {
				lines = append(lines, omittedLine(omitted, model.DetailedListPath))
			}
		}
	}
	if opts.IncludeSkipped {
		lines = append(lines, fmt.Sprintf("  Skipped items (%d)", len(model.Skipped)))
		if len(model.Skipped) == 0 {
			lines = append(lines, fmt.Sprintf("    %sNo skipped cleanup paths reported.", markerPrefix(opts, "loaded")))
		} else {
			entryCount := cappedEntryCountFor(len(model.Skipped), opts.EntryLimit)
			for _, skipped := range model.Skipped[:entryCount] {
				if opts.Compact {
					label := compactPathLabel(skipped.Path)
					if opts.Expanded {
						label = skipped.Path
					}
					line := fmt.Sprintf("    [boundary] %s (rule: %s, reason: %s, recoverable: %t)",
						label, skipped.Rule, skipped.Reason.Code, skipped.Reason.Recoverable)
					if opts.Expanded {
						line = fmt.Sprintf("    [boundary] %s (status: %s, rule: %s, reason: %s, recoverable: %t)",
							label, presentation.skippedLabel, skipped.Rule, skipped.Reason.Code, skipped.Reason.Recoverable)
					}
					lines = append(lines, line)
					if opts.Expanded && skipped.Reason.Message != "" {
						lines = append(lines, fmt.Sprintf("      %s", skipped.Reason.Message))
					}
					continue
				}
				lines = append(lines, fmt.Sprintf("    %s (%srule: %s, reason: %s, recoverable: %t)",
					skipped.Path, statusLabel(presentation.skippedLabel), skipped.Rule, skipped.Reason.Code, skipped.Reason.Recoverable))
				if opts.Expanded && skipped.Reason.Message != "" {
					lines = append(lines, fmt.Sprintf("      %s", skipped.Reason.Message))
				}
			}
			if omitted := len(model.Skipped) - entryCount; omitted > 0 {
				lines = append(lines, omittedLine(omitted, model.DetailedListPath))
			}
		}
	}
	if opts.IncludeReview {
		userOpportunities, omitted := categorizedOpportunities(model.Opportunities, opts.EntryLimit, func(opportunity Opportunity) bool {
			return categoryReportGroup(normalizedOpportunityCategory(opportunity.Category)) == ReportCategoryUserEssentials
		})
		if len(userOpportunities) > 0 {
			lines = append(lines, fmt.Sprintf("  Skipped by default: %d user-temp opportunity item(s)", len(userOpportunities)+omitted))
			lines = append(lines, opportunityLines(userOpportunities, opts)...)
			if omitted > 0 {
				lines = append(lines, omittedLine(omitted, model.DetailedListPath))
			}
		}
		for _, skipped := range model.SkippedByDefault {
			line := fmt.Sprintf("    %s%s", markerPrefix(opts, "opportunity"), skipped.Name)
			if skipped.Path != "" {
				path := compactPathLabel(skipped.Path)
				if opts.Expanded {
					path = skipped.Path
				}
				line += fmt.Sprintf(" - %s", path)
			}
			if skipped.Bytes > 0 {
				line += fmt.Sprintf(" (%s, status: skipped by default, not counted as Potential space)", reportFormatBytes(opts, skipped.Bytes))
			} else {
				line += " (status: skipped by default, not counted as Potential space)"
			}
			lines = append(lines, line)
			if opts.Expanded && skipped.Reason != "" {
				lines = append(lines, fmt.Sprintf("      %s", skipped.Reason))
			}
		}
	}
	if opts.IncludeIncompleteInspections {
		for _, incomplete := range model.IncompleteOpportunityInspections {
			if categoryReportGroup(normalizedOpportunityCategory(incomplete.Category)) == ReportCategoryUserEssentials {
				lines = append(lines, incompleteInspectionLine(incomplete, opts))
			}
		}
	}
	return lines
}

func browserReportLines(model PreviewReadModel, opts PreviewReportCategoryOptions) []string {
	var lines []string
	if opts.IncludeReview {
		browserOpportunities, omitted := categorizedOpportunities(model.Opportunities, opts.EntryLimit, func(opportunity Opportunity) bool {
			return categoryReportGroup(normalizedOpportunityCategory(opportunity.Category)) == ReportCategoryBrowsers
		})
		if len(browserOpportunities) > 0 {
			lines = append(lines, fmt.Sprintf("  Skipped by default: %d browser opportunity item(s)", len(browserOpportunities)+omitted))
			lines = append(lines, opportunityLines(browserOpportunities, opts)...)
			if omitted > 0 {
				lines = append(lines, omittedLine(omitted, model.DetailedListPath))
			}
		}
		if len(model.RunningApplicationSkips) > 0 {
			lines = append(lines, fmt.Sprintf("  Running application skips (%d)", len(model.RunningApplicationSkips)))
			for _, skipped := range model.RunningApplicationSkips {
				line := fmt.Sprintf("    %s%s", markerPrefix(opts, "boundary"), skipped.Name)
				if skipped.Application != "" {
					line += fmt.Sprintf(" (%s)", skipped.Application)
				}
				if skipped.Path != "" {
					path := compactPathLabel(skipped.Path)
					if opts.Expanded {
						path = skipped.Path
					}
					line += fmt.Sprintf(" - %s", path)
				}
				if opts.Compact && !opts.Expanded {
					line += " (running skip, not executable here)"
				} else {
					line += " (status: running skip, not executable here)"
				}
				lines = append(lines, line)
				if opts.Expanded && skipped.Reason != "" {
					lines = append(lines, fmt.Sprintf("      %s", skipped.Reason))
				}
			}
		}
	}
	if opts.IncludeIncompleteInspections {
		for _, incomplete := range model.IncompleteOpportunityInspections {
			if categoryReportGroup(normalizedOpportunityCategory(incomplete.Category)) == ReportCategoryBrowsers {
				lines = append(lines, incompleteInspectionLine(incomplete, opts))
			}
		}
	}
	if opts.IncludeErrors {
		for _, err := range model.Errors {
			if isBrowserDiagnostic(err) {
				lines = append(lines, fmt.Sprintf("    %sBrowser inspection diagnostic: %s (status: inspection incomplete, recoverable: %t)", markerPrefix(opts, "diagnostic"), err.Code, err.Recoverable))
				if opts.Expanded && err.Message != "" {
					lines = append(lines, fmt.Sprintf("      %s", err.Message))
				}
			}
		}
	}
	return lines
}

func developerToolReportLines(model PreviewReadModel, opts PreviewReportCategoryOptions) []string {
	lines := categorizedOpportunityReportLines(model, opts, ReportCategoryDeveloperTools, "developer-tool")
	if len(model.ReviewSuggestions) == 0 {
		return lines
	}
	lines = append(lines,
		fmt.Sprintf("  Review suggestions (%d)", len(model.ReviewSuggestions)),
		fmt.Sprintf("    %s", ReviewSuggestionSafetyNote),
	)
	for _, suggestion := range model.ReviewSuggestions {
		if opts.Compact {
			lines = append(lines, fmt.Sprintf("    [review] %s (Review suggestion) (status: Review suggestion)", suggestion.Label))
		} else {
			lines = append(lines, fmt.Sprintf("    %s (status: Review suggestion)", suggestion.Label))
		}
		if opts.Expanded && suggestion.Command != "" {
			lines = append(lines, fmt.Sprintf("      Command: %s", suggestion.Command))
		}
		if opts.Expanded && suggestion.CachePath != "" {
			lines = append(lines, fmt.Sprintf("      Cache: %s", suggestion.CachePath))
		}
		if opts.Expanded && suggestion.Command == "" && suggestion.NextStep != "" {
			lines = append(lines, fmt.Sprintf("      %s", suggestion.NextStep))
		}
	}
	return lines
}

// applicationReportLines renders skipped-by-default Application cache
// opportunities whose Report category is Applications (non-editor end-user
// application caches such as obsidian_cache). It mirrors developerToolReportLines
// without Review suggestions: Applications categories have no tool-query probe.
func applicationReportLines(model PreviewReadModel, opts PreviewReportCategoryOptions) []string {
	return categorizedOpportunityReportLines(model, opts, ReportCategoryApplications, "application")
}

func categorizedOpportunityReportLines(
	model PreviewReadModel,
	opts PreviewReportCategoryOptions,
	reportCategory ReportCategory,
	opportunityKind string,
) []string {
	var lines []string
	opportunities, omitted := categorizedOpportunities(model.Opportunities, opts.EntryLimit, func(opportunity Opportunity) bool {
		return categoryReportGroup(normalizedOpportunityCategory(opportunity.Category)) == reportCategory
	})
	if len(opportunities) > 0 {
		lines = append(lines, fmt.Sprintf("  Skipped by default: %d %s opportunity item(s)", len(opportunities)+omitted, opportunityKind))
		lines = append(lines, opportunityLines(opportunities, opts)...)
		if omitted > 0 {
			lines = append(lines, omittedLine(omitted, model.DetailedListPath))
		}
	}
	if opts.IncludeIncompleteInspections {
		for _, incomplete := range model.IncompleteOpportunityInspections {
			if categoryReportGroup(normalizedOpportunityCategory(incomplete.Category)) != reportCategory {
				continue
			}
			lines = append(lines, incompleteInspectionLine(incomplete, opts))
		}
	}
	return lines
}

func projectArtifactReportLines(model PreviewReadModel, opts PreviewReportCategoryOptions) []string {
	if len(model.ReviewClues) == 0 {
		return nil
	}
	lines := []string{fmt.Sprintf("  Review clues (%d)", len(model.ReviewClues))}
	for _, clue := range model.ReviewClues {
		line := fmt.Sprintf("    %s%s (review only) (status: Review clue)", markerPrefix(opts, "review"), clue.Name)
		if clue.Path != "" {
			path := compactPathLabel(clue.Path)
			if opts.Expanded {
				path = clue.Path
			}
			line = fmt.Sprintf("    %s%s - %s (review only) (status: Review clue)", markerPrefix(opts, "review"), clue.Name, path)
		}
		lines = append(lines, line)
		if opts.Expanded && clue.Details != "" {
			lines = append(lines, fmt.Sprintf("      %s", clue.Details))
		}
	}
	return lines
}

func protectionReportLines(model PreviewReadModel, opts PreviewReportCategoryOptions) []string {
	var lines []string
	if opts.IncludeReview {
		lines = append(lines, "  Protection rules")
		if len(model.ProtectionRules) == 0 {
			if opts.Compact {
				lines = append(lines, "    [loaded] Protection rules not reported.")
			} else {
				lines = append(lines, "    No default-enabled protection rules were reported.")
			}
		} else {
			for _, rule := range model.ProtectionRules {
				if rule.UserDefined {
					path := rule.Path
					if opts.Compact && !opts.Expanded {
						path = compactPathLabel(rule.Path)
					}
					lines = append(lines, fmt.Sprintf("    %s%s (user-defined Protection rule)", markerPrefix(opts, "boundary"), path))
					continue
				}
				lines = append(lines, fmt.Sprintf("    %s%s: %s", markerPrefix(opts, "boundary"), rule.ID, rule.Description))
			}
		}
	}
	if opts.IncludeProtectionDiagnostics && len(model.ProtectionDiagnostics) > 0 {
		lines = append(lines, fmt.Sprintf("  Protection diagnostics (%d)", len(model.ProtectionDiagnostics)))
		for _, diagnostic := range model.ProtectionDiagnostics {
			source := diagnostic.Source
			if opts.Compact && !opts.Expanded {
				source = compactPathLabel(diagnostic.Source)
			}
			line := fmt.Sprintf("    %s%s (status: Protection diagnostic, source: %s", markerPrefix(opts, "diagnostic"), diagnostic.Code, source)
			if diagnostic.Line > 0 {
				line += fmt.Sprintf(", line %d", diagnostic.Line)
			}
			line += fmt.Sprintf(", recoverable: %t)", diagnostic.Recoverable)
			lines = append(lines, line)
			if opts.Expanded && diagnostic.Message != "" {
				lines = append(lines, fmt.Sprintf("      %s", diagnostic.Message))
			}
		}
	}
	return lines
}

func summaryReportLines(model PreviewReadModel, opts PreviewReportCategoryOptions, presentation previewReportPresentation) []string {
	lines := []string{
		fmt.Sprintf("  Potential space: %s", reportFormatBytes(opts, model.PotentialSpaceBytes)),
		fmt.Sprintf("  Observed opportunity bytes: %s (not counted as Potential space)", reportFormatBytes(opts, model.OpportunityObservedBytes)),
	}
	if opts.PreviewSummary {
		lines = []string{
			"  Dry-run complete",
			"  No files changed.",
			fmt.Sprintf("  Potential space: %s", reportFormatBytes(opts, model.PotentialSpaceBytes)),
			fmt.Sprintf("  Observed opportunity bytes: %s (not counted as Potential space)", reportFormatBytes(opts, model.OpportunityObservedBytes)),
		}
	}
	if model.DetailedListPath != "" {
		lines = append(lines, fmt.Sprintf("  Detailed candidate list: %s", model.DetailedListPath))
	}
	if opts.IncludeErrors {
		lines = append(lines, fmt.Sprintf("  Inspection errors (%d)", len(model.Errors)))
		if len(model.Errors) == 0 {
			lines = append(lines, fmt.Sprintf("    %sNo recoverable inspection errors reported.", markerPrefix(opts, "loaded")))
		} else {
			entryCount := cappedEntryCountFor(len(model.Errors), opts.EntryLimit)
			for _, err := range model.Errors[:entryCount] {
				if opts.Compact {
					label := compactPathLabel(err.Path)
					if opts.Expanded {
						label = err.Path
					}
					lines = append(lines, fmt.Sprintf("    [diagnostic] %s (rule: %s, error: %s, recoverable: %t)",
						label, err.Rule, err.Code, err.Recoverable))
					if opts.Expanded && err.Message != "" {
						lines = append(lines, fmt.Sprintf("      %s", err.Message))
					}
					continue
				}
				lines = append(lines, fmt.Sprintf("    %s (%srule: %s, error: %s, recoverable: %t)",
					err.Path, statusLabel(presentation.inspectionErrorLabel), err.Rule, err.Code, err.Recoverable))
				if opts.Expanded && err.Message != "" {
					lines = append(lines, fmt.Sprintf("      %s", err.Message))
				}
			}
			if omitted := len(model.Errors) - entryCount; omitted > 0 {
				lines = append(lines, omittedLine(omitted, model.DetailedListPath))
			}
		}
	}
	if opts.PreviewSummary {
		lines = append(lines, fmt.Sprintf("  Default candidates: %d | Skipped: %d | Diagnostics: %d", model.CandidateCount, model.SkippedCount, len(model.Errors)))
	} else {
		lines = append(lines, fmt.Sprintf("  Candidates: %d, skipped: %d, errors: %d.", model.CandidateCount, model.SkippedCount, len(model.Errors)))
	}
	if model.Summary != "" {
		lines = append(lines, fmt.Sprintf("  %s", model.Summary))
	}
	return lines
}

func categorizedOpportunities(opportunities []Opportunity, limit int, include func(Opportunity) bool) ([]Opportunity, int) {
	var selected []Opportunity
	for _, opportunity := range opportunities {
		if include(opportunity) {
			selected = append(selected, opportunity)
		}
	}
	entryCount := cappedEntryCountFor(len(selected), limit)
	return selected[:entryCount], len(selected) - entryCount
}

func opportunityLines(opportunities []Opportunity, opts PreviewReportCategoryOptions) []string {
	lines := make([]string, 0, len(opportunities))
	for _, opportunity := range opportunities {
		category := normalizedOpportunityCategory(opportunity.Category)
		var line string
		if opportunity.BrowserCache != nil {
			line = fmt.Sprintf("    %s%s browser cache (%s, category: %s, profiles: %d",
				markerPrefix(opts, "opportunity"),
				applicationDisplayName(opportunity.BrowserCache.Browser),
				reportFormatBytes(opts, opportunity.Bytes),
				category,
				opportunity.BrowserCache.ProfileCount)
		} else {
			label := opportunity.Path
			if opts.Compact && !opts.Expanded {
				label = compactPathLabel(opportunity.Path)
			}
			line = fmt.Sprintf("    %s%s (%s, category: %s",
				markerPrefix(opts, "opportunity"), label, reportFormatBytes(opts, opportunity.Bytes), category)
		}
		if category == OpportunityCategoryUserTemp {
			line += fmt.Sprintf(", latest modified: %s, idle days: %d",
				opportunity.LatestModifiedAt.UTC().Format(time.RFC3339), opportunity.IdleDays)
		}
		line += fmt.Sprintf(", status: %s, reason: %s, not counted as Potential space)",
			opportunity.Status, opportunity.Reason)
		lines = append(lines, line)
	}
	return lines
}

func incompleteInspectionLine(incomplete IncompleteOpportunityInspection, opts PreviewReportCategoryOptions) string {
	path := incomplete.Path
	if opts.Compact && !opts.Expanded {
		path = compactPathLabel(incomplete.Path)
	}
	return fmt.Sprintf("    %s%s (category: %s, status: inspection incomplete, reason: %s, recoverable: %t)",
		markerPrefix(opts, "diagnostic"), path, normalizedOpportunityCategory(incomplete.Category), incomplete.Reason.Code, incomplete.Reason.Recoverable)
}

func isBrowserDiagnostic(issue StructuredIssue) bool {
	if issue.Rule == "browser_review" || issue.Rule == OpportunityCategoryBrowserCache {
		return true
	}
	// Legacy browser diagnostics used the detection code with browser_review rule;
	// keep code-only matches only when the rule is empty (older fixtures).
	return issue.Code == runningApplicationDetectionIssueCode && issue.Rule == ""
}

func cappedEntryCountFor(count, limit int) int {
	if count > limit {
		return limit
	}
	return count
}

func omittedLine(omitted int, detailedListPath string) string {
	line := fmt.Sprintf("    %d omitted.", omitted)
	if detailedListPath != "" {
		line += fmt.Sprintf(" See detailed candidate list for full path detail: %s", detailedListPath)
	}
	return line
}

func cappedEntryCount(count int) int {
	if count > previewReportSectionEntryLimit {
		return previewReportSectionEntryLimit
	}
	return count
}

func writeOmittedLine(builder *strings.Builder, count int, detailedListPath string) {
	omitted := count - previewReportSectionEntryLimit
	if omitted <= 0 {
		return
	}
	builder.WriteString(fmt.Sprintf("  %d omitted.", omitted))
	if detailedListPath != "" {
		builder.WriteString(fmt.Sprintf(" See detailed candidate list for full path detail: %s", detailedListPath))
	}
	builder.WriteString("\n")
}

func statusLabel(label string) string {
	if label == "" {
		return ""
	}
	return fmt.Sprintf("status: %s, ", label)
}

func markerPrefix(opts PreviewReportCategoryOptions, marker string) string {
	if !opts.Compact {
		return ""
	}
	return fmt.Sprintf("[%s] ", marker)
}

func compactPathLabel(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return trimmed
	}
	trimmed = strings.TrimPrefix(trimmed, `\\?\`)
	trimmed = strings.TrimRight(trimmed, `\/`)
	lastSeparator := strings.LastIndexAny(trimmed, `\/`)
	if lastSeparator >= 0 && lastSeparator+1 < len(trimmed) {
		return trimmed[lastSeparator+1:]
	}
	return trimmed
}

func formatBytes(bytes int64) string {
	return fmt.Sprintf("%d bytes", bytes)
}

func reportFormatBytes(opts PreviewReportCategoryOptions, bytes int64) string {
	if opts.ByteFormatter != nil {
		return opts.ByteFormatter(bytes)
	}
	return formatBytes(bytes)
}
