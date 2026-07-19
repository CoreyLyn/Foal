package clean_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

func vscodeAllowlistedRoots() []string {
	return []string{
		"Cache",
		"CachedData",
		"CachedExtensionVSIXs",
		"Code Cache",
		"GPUCache",
		"DawnGraphiteCache",
		"DawnWebGPUCache",
	}
}

func writeVSCodeRoot(t *testing.T, roamingAppData string, rootContents map[string]string) string {
	t.Helper()
	codeRoot := filepath.Join(roamingAppData, "Code")
	if err := os.MkdirAll(codeRoot, 0700); err != nil {
		t.Fatal(err)
	}
	for name, contents := range rootContents {
		path := filepath.Join(codeRoot, name)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		if contents != "" {
			if err := os.WriteFile(filepath.Join(path, "data.bin"), []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return codeRoot
}

func idleVSCodeDetector() func(context.Context) []clean.RunningApplicationState {
	return func(context.Context) []clean.RunningApplicationState {
		// Omit browsers so default browser discovery does not inspect the host.
		return []clean.RunningApplicationState{
			{Application: clean.ApplicationVisualStudioCode, State: clean.RunningApplicationStateIdle},
		}
	}
}

func TestDryRunReportsIdleVSCodeCacheRootsAsIndependentOpportunities(t *testing.T) {
	roaming := t.TempDir()
	contents := map[string]string{}
	var wantBytes int64
	for _, root := range vscodeAllowlistedRoots() {
		payload := root + "-bytes"
		contents[root] = payload
		wantBytes += int64(len(payload))
	}
	// Sensitive and decoy directories must never become opportunities.
	for _, decoy := range []string{
		"User", "CachedProfilesData", "workspaceStorage", "globalStorage", "Backups",
		"extensions", "Service Worker", "Local Storage", "Session Storage", "WebStorage",
		"Network", "Cookies", "logs", "Crashpad", "MyCache", "cache-temp",
	} {
		contents[decoy] = "must-not-count"
	}
	codeRoot := writeVSCodeRoot(t, roaming, contents)
	recorder := &recordingHistoryRecorder{}

	result := clean.DryRun(context.Background(), clean.Options{
		HistoryRecorder: recorder,
		DetailedListDir: filepath.Join(t.TempDir(), "Foal", "history"),
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: t.TempDir(),
		},
		DetectRunningApplications: idleVSCodeDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	vscodeOpps := opportunitiesForCategory(result, clean.OpportunityCategoryVSCodeCache)
	if len(vscodeOpps) != len(vscodeAllowlistedRoots()) {
		t.Fatalf("vscode opportunities = %#v, want one per allowlisted root", vscodeOpps)
	}
	seen := make(map[string]bool)
	var total int64
	for i, opportunity := range vscodeOpps {
		if opportunity.Status != clean.OpportunityStatus || opportunity.Reason != clean.OpportunityReason {
			t.Fatalf("opportunity[%d] status/reason = %#v", i, opportunity)
		}
		base := filepath.Base(opportunity.Path)
		if !strings.HasPrefix(opportunity.Path, codeRoot) {
			t.Fatalf("opportunity path %q not under Code root", opportunity.Path)
		}
		seen[base] = true
		total += opportunity.Bytes
	}
	for _, root := range vscodeAllowlistedRoots() {
		if !seen[root] {
			t.Fatalf("missing allowlisted root %q", root)
		}
	}
	if total != wantBytes || result.Totals.OpportunityObservedBytes != wantBytes || result.Totals.CandidateBytes != 0 {
		t.Fatalf("totals = %#v total=%d want observed-only %d", result.Totals, total, wantBytes)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(encoded)
	wantObserved := `"opportunity_observed_bytes":` + string(mustJSON(wantBytes))
	for _, want := range []string{`"category":"vscode_cache"`, wantObserved} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("JSON missing %q: %s", want, jsonText)
		}
	}
	for _, forbidden := range []string{
		`\User\`, `/User/`, "workspaceStorage", "globalStorage", "Backups",
		`\extensions\`, "Service Worker", "Local Storage", "Cookies", "Crashpad",
		"MyCache", "cache-temp", "move_to_recycle_bin",
	} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("JSON contains excluded data %q: %s", forbidden, jsonText)
		}
	}

	model := clean.NewPreviewReadModel(result)
	report := clean.RenderPreviewReport(model)
	for _, want := range []string{
		"Developer tools",
		"developer-tool opportunity",
		"category: vscode_cache",
		"Observed opportunity bytes:",
		"Potential space: 0 bytes",
		"CachedExtensionVSIXs holds downloaded extension packages",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("human report missing %q:\n%s", want, report)
		}
	}

	if len(recorder.sessions) != 1 ||
		recorder.sessions[0].Aggregate.OpportunityCount != len(vscodeOpps) ||
		recorder.sessions[0].Aggregate.OpportunityObservedBytes != wantBytes ||
		len(recorder.items) != 0 {
		t.Fatalf("history = %#v / %#v, want aggregate-only privacy", recorder.sessions, recorder.items)
	}
	if strings.Contains(recorder.encoded, codeRoot) || strings.Contains(recorder.encoded, "CachedData") {
		t.Fatalf("history leaked editor path: %s", recorder.encoded)
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TestDryRunVSCodeMissingRootIsSilentAbsence(t *testing.T) {
	result := clean.DryRun(context.Background(), clean.Options{
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: t.TempDir(),
		},
		DetectRunningApplications: idleVSCodeDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryVSCodeCache {
			t.Fatalf("unexpected vscode opportunity: %#v", opportunity)
		}
	}
	if len(result.IncompleteOpportunityInspections) != 0 {
		t.Fatalf("incomplete = %#v, want empty for missing Code root", result.IncompleteOpportunityInspections)
	}
}

func TestDryRunVSCodeRunningSkipsWithoutMeasuring(t *testing.T) {
	roaming := t.TempDir()
	writeVSCodeRoot(t, roaming, map[string]string{"Cache": "cache", "CachedData": "data"})
	result := clean.DryRun(context.Background(), clean.Options{
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationVisualStudioCode, State: clean.RunningApplicationStateRunning},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryVSCodeCache {
			t.Fatalf("running VS Code produced opportunity: %#v", opportunity)
		}
	}
	if result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("observed bytes = %d, want 0", result.Totals.OpportunityObservedBytes)
	}
	model := clean.NewPreviewReadModel(result)
	report := clean.RenderPreviewReport(model)
	if !strings.Contains(report, "Visual Studio Code") {
		t.Fatalf("report missing VS Code running skip:\n%s", report)
	}
}

func TestDryRunVSCodeUnknownFailsClosed(t *testing.T) {
	roaming := t.TempDir()
	writeVSCodeRoot(t, roaming, map[string]string{"Cache": "cache"})
	result := clean.DryRun(context.Background(), clean.Options{
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationVisualStudioCode, State: clean.RunningApplicationStateUnknown, Message: "snapshot failed"},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryVSCodeCache {
			t.Fatalf("unknown state produced opportunity: %#v", opportunity)
		}
	}
	found := false
	for _, err := range result.Errors {
		if err.Code == "running_application_detection_unknown" {
			found = true
		}
	}
	if !found {
		t.Fatalf("errors = %#v, want unknown diagnostic", result.Errors)
	}
}

func TestDryRunVSCodePostRunningDiscardsMeasuredRoots(t *testing.T) {
	roaming := t.TempDir()
	writeVSCodeRoot(t, roaming, map[string]string{"Cache": "cache", "GPUCache": "gpu"})
	calls := 0
	result := clean.DryRun(context.Background(), clean.Options{
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			calls++
			state := clean.RunningApplicationStateIdle
			if calls > 1 {
				state = clean.RunningApplicationStateRunning
			}
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationVisualStudioCode, State: state},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryVSCodeCache {
			t.Fatalf("post-running kept opportunity: %#v", opportunity)
		}
	}
	if result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("observed bytes = %d after post-running discard", result.Totals.OpportunityObservedBytes)
	}
}

func TestDryRunVSCodeProtectionSuppressesRootBeforeTotals(t *testing.T) {
	roaming := t.TempDir()
	codeRoot := writeVSCodeRoot(t, roaming, map[string]string{
		"Cache":      "cache",
		"CachedData": "data",
		"GPUCache":   "gpu",
	})
	protected := filepath.Join(codeRoot, "CachedData")
	result := clean.DryRun(context.Background(), clean.Options{
		Validator: pathsafe.NewValidator([]string{protected}),
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		DetectRunningApplications: idleVSCodeDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryVSCodeCache && opportunity.Path == protected {
			t.Fatalf("protected root leaked: %#v", opportunity)
		}
	}
	var vscodeCount int
	var observed int64
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryVSCodeCache {
			vscodeCount++
			observed += opportunity.Bytes
		}
	}
	if vscodeCount != 2 || observed != 8 {
		t.Fatalf("vscodeCount=%d observed=%d, want 2 siblings totaling 8", vscodeCount, observed)
	}
}

func opportunitiesForCategory(result clean.Result, category string) []clean.Opportunity {
	var out []clean.Opportunity
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == category {
			out = append(out, opportunity)
		}
	}
	return out
}

func TestDryRunVSCodeOptInConvertsRootsToCandidates(t *testing.T) {
	roaming := t.TempDir()
	writeVSCodeRoot(t, roaming, map[string]string{
		"Cache":                "cache",
		"CachedExtensionVSIXs": "vsix-pkg",
	})
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.OpportunityCategoryVSCodeCache},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: t.TempDir(),
		},
		DetectRunningApplications: idleVSCodeDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if len(opportunitiesForCategory(result, clean.OpportunityCategoryVSCodeCache)) != 0 {
		t.Fatalf("opted-in category still listed as opportunity: %#v", result.Opportunities)
	}
	if len(result.OptInCandidates) != 2 {
		t.Fatalf("opt-in candidates = %#v, want 2", result.OptInCandidates)
	}
	var reclaimable int64
	for _, candidate := range result.OptInCandidates {
		if candidate.Category != clean.OpportunityCategoryVSCodeCache {
			t.Fatalf("candidate category = %q", candidate.Category)
		}
		if candidate.PlannedAction != string(clean.DeletionActionDeletePermanently) {
			t.Fatalf("planned action = %q, want delete_permanently", candidate.PlannedAction)
		}
		reclaimable += candidate.Bytes
	}
	if result.Totals.OptInReclaimableBytes != reclaimable {
		t.Fatalf("totals = %#v, reclaimable=%d", result.Totals, reclaimable)
	}
	if result.Totals.OpportunityObservedBytes != 0 {
		// Non-VS Code observations may exist; VS Code must not contribute after opt-in.
		for _, opportunity := range result.Opportunities {
			if opportunity.Category == clean.OpportunityCategoryVSCodeCache {
				t.Fatalf("vscode still in opportunities: %#v", opportunity)
			}
		}
	}
	model := clean.NewPreviewReadModel(result)
	foundNotice := false
	for _, notice := range model.Notices {
		if notice.Kind == "opt_in_impact" && strings.Contains(notice.Message, "CachedExtensionVSIXs") {
			foundNotice = true
		}
	}
	if !foundNotice {
		t.Fatalf("notices = %#v, want VSIX impact notice", model.Notices)
	}
}

func TestExecuteWithoutVSCodeOptInSkipsDetection(t *testing.T) {
	roaming := t.TempDir()
	writeVSCodeRoot(t, roaming, map[string]string{"Cache": "cache"})
	detectorCalls := 0
	adapter := &recordingRecycleBinAdapter{}
	_ = clean.Execute(context.Background(), clean.Options{
		Rules:             []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		RecycleBinAdapter: adapter,
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			detectorCalls++
			return idleVSCodeDetector()(context.Background())
		},
	})
	if detectorCalls != 0 {
		t.Fatalf("DetectRunningApplications called %d times without opt-in", detectorCalls)
	}
	if len(adapter.paths) != 0 {
		t.Fatalf("adapter paths = %v", adapter.paths)
	}
}

func TestExecuteOptInVSCodeCacheCleansWhenIdle(t *testing.T) {
	roaming := t.TempDir()
	codeRoot := writeVSCodeRoot(t, roaming, map[string]string{
		"Cache":      "cache-data",
		"CachedData": "compiled",
	})
	cachePath := filepath.Join(codeRoot, "Cache")
	cachedDataPath := filepath.Join(codeRoot, "CachedData")
	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	recorder := &recordingHistoryRecorder{}

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		Rules:                  []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		RecycleBinAdapter:      recycle,
		PermanentRemover:       permanent,
		AllowPermanentDeletion: true,
		HistoryRecorder:        recorder,
		OptIn:                  []string{clean.OpportunityCategoryVSCodeCache},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		DetectRunningApplications: idleVSCodeDetector(),
	})

	if result.Totals.OptInDeletedCount != 2 {
		t.Fatalf("OptInDeletedCount = %d, want 2; result=%#v", result.Totals.OptInDeletedCount, result)
	}
	if len(recycle.paths) != 0 {
		t.Fatalf("vscode permanent must not use Recycle Bin: %v", recycle.paths)
	}
	found := map[string]bool{}
	for _, path := range permanent.paths {
		found[path] = true
	}
	if !found[cachePath] || !found[cachedDataPath] {
		t.Fatalf("permanent paths = %v, want Cache and CachedData", permanent.paths)
	}
	for _, item := range result.Deleted {
		if item.Action != string(clean.DeletionActionDeletePermanently) {
			t.Fatalf("deleted action = %q", item.Action)
		}
	}
	// Opted-in execution history keeps path-bearing item records.
	if len(recorder.items) == 0 {
		t.Fatalf("history items empty, want path-bearing opt-in records")
	}
	for _, item := range recorder.items {
		if item.Path == cachePath || item.Path == cachedDataPath {
			if item.Action != string(clean.DeletionActionDeletePermanently) {
				t.Fatalf("history action = %q", item.Action)
			}
			return
		}
	}
	t.Fatalf("history items = %#v, want VS Code paths", recorder.items)
}

func TestExecuteOptInVSCodeFreshResolvesNotPreviewPaths(t *testing.T) {
	roamingPreview := t.TempDir()
	writeVSCodeRoot(t, roamingPreview, map[string]string{"Cache": "preview-only"})
	roamingExecute := t.TempDir()
	codeRoot := writeVSCodeRoot(t, roamingExecute, map[string]string{"GPUCache": "execute-root"})
	executePath := filepath.Join(codeRoot, "GPUCache")
	permanent := &recordingPermanentRemover{}

	// Preview would see Cache under roamingPreview; execute must use fresh options.
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		Rules:                  []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		PermanentRemover:       permanent,
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.OpportunityCategoryVSCodeCache},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roamingExecute,
		},
		DetectRunningApplications: idleVSCodeDetector(),
	})
	if result.Totals.OptInDeletedCount != 1 {
		t.Fatalf("OptInDeletedCount = %d, want 1", result.Totals.OptInDeletedCount)
	}
	if len(permanent.paths) != 1 || permanent.paths[0] != executePath {
		t.Fatalf("permanent paths = %v, want fresh GPUCache %q", permanent.paths, executePath)
	}
}

func TestExecuteOptInVSCodePermanentExcludesRecycleBinCapacity(t *testing.T) {
	// Permanent editor caches are excluded from Recycle Bin capacity budgets.
	// Tiny capacity must not block authorized permanent deletion.
	roaming := t.TempDir()
	codeRoot := writeVSCodeRoot(t, roaming, map[string]string{"Cache": string(make([]byte, 64))})
	cachePath := filepath.Join(codeRoot, "Cache")
	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	probe := func(path string) (clean.RecycleBinVolumeConfig, error) {
		return clean.RecycleBinVolumeConfig{
			Volume:       filepath.VolumeName(path),
			NukeOnDelete: false,
			MaxCapacity:  1,
			CurrentUsage: 0,
		}, nil
	}
	result := clean.Execute(context.Background(), clean.Options{
		Rules:                   []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		RecycleBinAdapter:       recycle,
		PermanentRemover:        permanent,
		AllowPermanentDeletion:  true,
		RecycleBinCapacityProbe: probe,
		OptIn:                   []string{clean.OpportunityCategoryVSCodeCache},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		DetectRunningApplications: idleVSCodeDetector(),
	})
	if len(recycle.paths) != 0 {
		t.Fatalf("recycle adapter called for permanent category: %v", recycle.paths)
	}
	if result.Totals.OptInDeletedCount != 1 || len(permanent.paths) != 1 || permanent.paths[0] != cachePath {
		t.Fatalf("permanent delete failed under tiny capacity: totals=%#v paths=%v", result.Totals, permanent.paths)
	}
	if result.Totals.PermanentlyDeletedBytes == 0 {
		t.Fatalf("permanently_deleted_bytes = 0")
	}
}

func TestNormalizedOptInSetVSCodeAndDevCaches(t *testing.T) {
	enabled, invalid, _ := clean.NormalizedOptInSet([]string{"vscode_cache"})
	if len(invalid) != 0 || !enabled[clean.OpportunityCategoryVSCodeCache] {
		t.Fatalf("vscode_cache opt-in = %#v %#v", enabled, invalid)
	}
	enabled, invalid, _ = clean.NormalizedOptInSet([]string{"cursor_cache"})
	if len(invalid) != 0 || !enabled[clean.OpportunityCategoryCursorCache] {
		t.Fatalf("cursor_cache opt-in = %#v %#v", enabled, invalid)
	}
	if enabled[clean.OpportunityCategoryVSCodeCache] {
		t.Fatalf("cursor_cache opt-in must not enable vscode_cache: %#v", enabled)
	}
	for _, id := range []string{
		clean.OpportunityCategoryVSCodeInsidersCache,
		clean.OpportunityCategoryVSCodiumCache,
		clean.OpportunityCategoryWindsurfCache,
		clean.OpportunityCategoryTraeCache,
	} {
		enabled, invalid, _ = clean.NormalizedOptInSet([]string{id})
		if len(invalid) != 0 || !enabled[id] {
			t.Fatalf("%s opt-in = %#v %#v", id, enabled, invalid)
		}
		if enabled[clean.OpportunityCategoryVSCodeCache] || enabled[clean.OpportunityCategoryCursorCache] {
			t.Fatalf("%s opt-in must not enable vscode/cursor: %#v", id, enabled)
		}
	}
	enabled, invalid, _ = clean.NormalizedOptInSet([]string{"dev-caches"})
	if len(invalid) != 0 ||
		!enabled[clean.OpportunityCategoryVSCodeCache] ||
		!enabled[clean.OpportunityCategoryCursorCache] ||
		!enabled[clean.OpportunityCategoryVSCodeInsidersCache] ||
		!enabled[clean.OpportunityCategoryVSCodiumCache] ||
		!enabled[clean.OpportunityCategoryWindsurfCache] ||
		!enabled[clean.OpportunityCategoryTraeCache] {
		t.Fatalf("dev-caches should enable all application-cache editors: %#v %#v", enabled, invalid)
	}
	enabled, invalid, _ = clean.NormalizedOptInSet([]string{"all"})
	if len(invalid) != 0 ||
		!enabled[clean.OpportunityCategoryVSCodeCache] ||
		!enabled[clean.OpportunityCategoryCursorCache] ||
		!enabled[clean.OpportunityCategoryVSCodeInsidersCache] ||
		!enabled[clean.OpportunityCategoryVSCodiumCache] ||
		!enabled[clean.OpportunityCategoryWindsurfCache] ||
		!enabled[clean.OpportunityCategoryTraeCache] {
		t.Fatalf("all should enable all application-cache editors: %#v %#v", enabled, invalid)
	}
	// cli-agents must not expand to Trae (or any editor cache).
	cliEnabled, cliInvalid, _ := clean.NormalizedOptInSet([]string{clean.CLIAgentCategoryGroup})
	if len(cliInvalid) != 0 || cliEnabled[clean.OpportunityCategoryTraeCache] {
		t.Fatalf("cli-agents must not enable trae_cache: %#v", cliEnabled)
	}
}

func editorAllowlistedRoots() []string {
	return []string{
		"Cache",
		"CachedData",
		"CachedExtensionVSIXs",
		"Code Cache",
		"GPUCache",
		"DawnGraphiteCache",
		"DawnWebGPUCache",
	}
}

func writeEditorUserDataRoot(t *testing.T, roamingAppData, appFolder string, rootContents map[string]string) string {
	t.Helper()
	userDataRoot := filepath.Join(roamingAppData, appFolder)
	if err := os.MkdirAll(userDataRoot, 0700); err != nil {
		t.Fatal(err)
	}
	for name, contents := range rootContents {
		path := filepath.Join(userDataRoot, name)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		if contents != "" {
			if err := os.WriteFile(filepath.Join(path, "data.bin"), []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return userDataRoot
}

func writeCursorRoot(t *testing.T, roamingAppData string, rootContents map[string]string) string {
	t.Helper()
	return writeEditorUserDataRoot(t, roamingAppData, "Cursor", rootContents)
}

func idleCursorDetector() func(context.Context) []clean.RunningApplicationState {
	return func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{Application: clean.ApplicationCursor, State: clean.RunningApplicationStateIdle},
		}
	}
}

func idleBothEditorsDetector() func(context.Context) []clean.RunningApplicationState {
	return func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{Application: clean.ApplicationVisualStudioCode, State: clean.RunningApplicationStateIdle},
			{Application: clean.ApplicationCursor, State: clean.RunningApplicationStateIdle},
		}
	}
}

func TestDryRunReportsIdleCursorCacheRootsAsIndependentOpportunities(t *testing.T) {
	roaming := t.TempDir()
	contents := map[string]string{}
	var wantBytes int64
	for _, root := range editorAllowlistedRoots() {
		payload := "cursor-" + root
		contents[root] = payload
		wantBytes += int64(len(payload))
	}
	for _, decoy := range []string{
		"User", "workspaceStorage", "globalStorage", "Backups",
		"extensions", "Service Worker", "Local Storage", "Session Storage", "WebStorage",
		"Network", "Cookies", "logs", "Crashpad", "MyCache", "cache-temp",
	} {
		contents[decoy] = "must-not-count"
	}
	cursorRoot := writeCursorRoot(t, roaming, contents)
	recorder := &recordingHistoryRecorder{}

	result := clean.DryRun(context.Background(), clean.Options{
		HistoryRecorder: recorder,
		DetailedListDir: filepath.Join(t.TempDir(), "Foal", "history"),
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: t.TempDir(),
		},
		DetectRunningApplications: idleCursorDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	cursorOpps := opportunitiesForCategory(result, clean.OpportunityCategoryCursorCache)
	if len(cursorOpps) != len(editorAllowlistedRoots()) {
		t.Fatalf("cursor opportunities = %#v, want one per allowlisted root", cursorOpps)
	}
	seen := make(map[string]bool)
	var total int64
	for _, opportunity := range cursorOpps {
		if opportunity.Status != clean.OpportunityStatus || opportunity.Reason != clean.OpportunityReason {
			t.Fatalf("opportunity status/reason = %#v", opportunity)
		}
		if !strings.HasPrefix(opportunity.Path, cursorRoot) {
			t.Fatalf("opportunity path %q not under Cursor root", opportunity.Path)
		}
		seen[filepath.Base(opportunity.Path)] = true
		total += opportunity.Bytes
	}
	for _, root := range editorAllowlistedRoots() {
		if !seen[root] {
			t.Fatalf("missing allowlisted root %q", root)
		}
	}
	if total != wantBytes || result.Totals.OpportunityObservedBytes != wantBytes || result.Totals.CandidateBytes != 0 {
		t.Fatalf("totals = %#v total=%d want observed-only %d", result.Totals, total, wantBytes)
	}
	if len(opportunitiesForCategory(result, clean.OpportunityCategoryVSCodeCache)) != 0 {
		t.Fatalf("cursor roots must not project as vscode_cache: %#v", result.Opportunities)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(encoded)
	if !strings.Contains(jsonText, `"category":"cursor_cache"`) {
		t.Fatalf("JSON missing cursor_cache: %s", jsonText)
	}
	for _, forbidden := range []string{
		`"category":"vscode_cache"`, "workspaceStorage", "globalStorage", "Backups",
		"Service Worker", "Local Storage", "Cookies", "Crashpad", "MyCache", "cache-temp",
	} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("JSON contains excluded data %q: %s", forbidden, jsonText)
		}
	}

	model := clean.NewPreviewReadModel(result)
	report := clean.RenderPreviewReport(model)
	for _, want := range []string{
		"Developer tools",
		"category: cursor_cache",
		"Observed opportunity bytes:",
		"Potential space: 0 bytes",
		"CachedExtensionVSIXs holds downloaded extension packages",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("human report missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "category: vscode_cache") {
		t.Fatalf("cursor-only dry-run should keep vscode summary separate/absent:\n%s", report)
	}

	if len(recorder.sessions) != 1 ||
		recorder.sessions[0].Aggregate.OpportunityCount != len(cursorOpps) ||
		recorder.sessions[0].Aggregate.OpportunityObservedBytes != wantBytes ||
		len(recorder.items) != 0 {
		t.Fatalf("history = %#v / %#v, want aggregate-only privacy", recorder.sessions, recorder.items)
	}
	if strings.Contains(recorder.encoded, cursorRoot) || strings.Contains(recorder.encoded, "CachedData") {
		t.Fatalf("history leaked editor path: %s", recorder.encoded)
	}
}

func TestDryRunCursorMissingRootIsSilentAbsence(t *testing.T) {
	result := clean.DryRun(context.Background(), clean.Options{
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: t.TempDir(),
		},
		DetectRunningApplications: idleCursorDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryCursorCache {
			t.Fatalf("unexpected cursor opportunity: %#v", opportunity)
		}
	}
	if len(result.IncompleteOpportunityInspections) != 0 {
		t.Fatalf("incomplete = %#v, want empty for missing Cursor root", result.IncompleteOpportunityInspections)
	}
}

func TestDryRunCursorRunningSkipsWithoutMeasuring(t *testing.T) {
	roaming := t.TempDir()
	writeCursorRoot(t, roaming, map[string]string{"Cache": "cache", "CachedData": "data"})
	result := clean.DryRun(context.Background(), clean.Options{
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationCursor, State: clean.RunningApplicationStateRunning},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryCursorCache {
			t.Fatalf("running Cursor produced opportunity: %#v", opportunity)
		}
	}
	if result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("observed bytes = %d, want 0", result.Totals.OpportunityObservedBytes)
	}
	model := clean.NewPreviewReadModel(result)
	report := clean.RenderPreviewReport(model)
	if !strings.Contains(report, "Cursor") {
		t.Fatalf("report missing Cursor running skip:\n%s", report)
	}
}

func TestDryRunCursorUnknownFailsClosed(t *testing.T) {
	roaming := t.TempDir()
	writeCursorRoot(t, roaming, map[string]string{"Cache": "cache"})
	result := clean.DryRun(context.Background(), clean.Options{
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationCursor, State: clean.RunningApplicationStateUnknown, Message: "snapshot failed"},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryCursorCache {
			t.Fatalf("unknown state produced opportunity: %#v", opportunity)
		}
	}
	found := false
	for _, err := range result.Errors {
		if err.Code == "running_application_detection_unknown" {
			found = true
		}
	}
	if !found {
		t.Fatalf("errors = %#v, want unknown diagnostic", result.Errors)
	}
}

func TestDryRunCursorPostRunningDiscardsMeasuredRoots(t *testing.T) {
	roaming := t.TempDir()
	writeCursorRoot(t, roaming, map[string]string{"Cache": "cache", "GPUCache": "gpu"})
	calls := 0
	result := clean.DryRun(context.Background(), clean.Options{
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			calls++
			state := clean.RunningApplicationStateIdle
			if calls > 1 {
				state = clean.RunningApplicationStateRunning
			}
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationCursor, State: state},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryCursorCache {
			t.Fatalf("post-running kept opportunity: %#v", opportunity)
		}
	}
	if result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("observed bytes = %d after post-running discard", result.Totals.OpportunityObservedBytes)
	}
}

func TestDryRunIndependentEditorGates(t *testing.T) {
	roaming := t.TempDir()
	writeVSCodeRoot(t, roaming, map[string]string{"Cache": "vscode-cache"})
	writeCursorRoot(t, roaming, map[string]string{"Cache": "cursor-cache"})

	// Running VS Code must not suppress idle Cursor.
	result := clean.DryRun(context.Background(), clean.Options{
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		BrowserCacheDiscoveryOptions:     clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: t.TempDir()},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationVisualStudioCode, State: clean.RunningApplicationStateRunning},
				{Application: clean.ApplicationCursor, State: clean.RunningApplicationStateIdle},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if len(opportunitiesForCategory(result, clean.OpportunityCategoryVSCodeCache)) != 0 {
		t.Fatalf("running VS Code still produced opportunities: %#v", result.Opportunities)
	}
	cursorOpps := opportunitiesForCategory(result, clean.OpportunityCategoryCursorCache)
	if len(cursorOpps) != 1 || filepath.Base(cursorOpps[0].Path) != "Cache" {
		t.Fatalf("idle Cursor opportunities = %#v, want Cache only", cursorOpps)
	}

	// Running Cursor must not suppress idle VS Code.
	result = clean.DryRun(context.Background(), clean.Options{
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		BrowserCacheDiscoveryOptions:     clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: t.TempDir()},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationVisualStudioCode, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationCursor, State: clean.RunningApplicationStateRunning},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if len(opportunitiesForCategory(result, clean.OpportunityCategoryCursorCache)) != 0 {
		t.Fatalf("running Cursor still produced opportunities: %#v", result.Opportunities)
	}
	vscodeOpps := opportunitiesForCategory(result, clean.OpportunityCategoryVSCodeCache)
	if len(vscodeOpps) != 1 || filepath.Base(vscodeOpps[0].Path) != "Cache" {
		t.Fatalf("idle VS Code opportunities = %#v, want Cache only", vscodeOpps)
	}
}

func TestDryRunCursorProtectionSuppressesRootBeforeTotals(t *testing.T) {
	roaming := t.TempDir()
	cursorRoot := writeCursorRoot(t, roaming, map[string]string{
		"Cache":      "cache",
		"CachedData": "data",
		"GPUCache":   "gpu",
	})
	protected := filepath.Join(cursorRoot, "CachedData")
	result := clean.DryRun(context.Background(), clean.Options{
		Validator: pathsafe.NewValidator([]string{protected}),
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		DetectRunningApplications: idleCursorDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryCursorCache && opportunity.Path == protected {
			t.Fatalf("protected root leaked: %#v", opportunity)
		}
	}
	var cursorCount int
	var observed int64
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryCursorCache {
			cursorCount++
			observed += opportunity.Bytes
		}
	}
	if cursorCount != 2 || observed != 8 {
		t.Fatalf("cursorCount=%d observed=%d, want 2 siblings totaling 8", cursorCount, observed)
	}
}

func TestDryRunCursorOptInConvertsRootsToCandidatesWithoutSelectingVSCode(t *testing.T) {
	roaming := t.TempDir()
	writeCursorRoot(t, roaming, map[string]string{
		"Cache":                "cache",
		"CachedExtensionVSIXs": "vsix-pkg",
	})
	writeVSCodeRoot(t, roaming, map[string]string{"Cache": "vscode-only"})
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.OpportunityCategoryCursorCache},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: t.TempDir(),
		},
		DetectRunningApplications: idleBothEditorsDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if len(opportunitiesForCategory(result, clean.OpportunityCategoryCursorCache)) != 0 {
		t.Fatalf("opted-in cursor still listed as opportunity: %#v", result.Opportunities)
	}
	vscodeOpps := opportunitiesForCategory(result, clean.OpportunityCategoryVSCodeCache)
	if len(vscodeOpps) != 1 {
		t.Fatalf("non-opted-in vscode should remain opportunity: %#v", result.Opportunities)
	}
	if len(result.OptInCandidates) != 2 {
		t.Fatalf("opt-in candidates = %#v, want 2 cursor roots", result.OptInCandidates)
	}
	for _, candidate := range result.OptInCandidates {
		if candidate.Category != clean.OpportunityCategoryCursorCache {
			t.Fatalf("candidate category = %q, want cursor_cache only", candidate.Category)
		}
		if candidate.PlannedAction != string(clean.DeletionActionDeletePermanently) {
			t.Fatalf("planned action = %q, want delete_permanently", candidate.PlannedAction)
		}
	}
	model := clean.NewPreviewReadModel(result)
	foundNotice := false
	for _, notice := range model.Notices {
		if notice.Kind == "opt_in_impact" && strings.Contains(notice.Message, "CachedExtensionVSIXs") {
			foundNotice = true
		}
	}
	if !foundNotice {
		t.Fatalf("notices = %#v, want VSIX impact notice", model.Notices)
	}
}

func TestExecuteWithoutCursorOptInSkipsDetection(t *testing.T) {
	roaming := t.TempDir()
	writeCursorRoot(t, roaming, map[string]string{"Cache": "cache"})
	detectorCalls := 0
	adapter := &recordingRecycleBinAdapter{}
	_ = clean.Execute(context.Background(), clean.Options{
		Rules:             []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		RecycleBinAdapter: adapter,
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			detectorCalls++
			return idleCursorDetector()(context.Background())
		},
	})
	if detectorCalls != 0 {
		t.Fatalf("DetectRunningApplications called %d times without opt-in", detectorCalls)
	}
	if len(adapter.paths) != 0 {
		t.Fatalf("adapter paths = %v", adapter.paths)
	}
}

func TestExecuteOptInCursorCacheCleansWhenIdle(t *testing.T) {
	roaming := t.TempDir()
	cursorRoot := writeCursorRoot(t, roaming, map[string]string{
		"Cache":      "cache-data",
		"CachedData": "compiled",
	})
	cachePath := filepath.Join(cursorRoot, "Cache")
	cachedDataPath := filepath.Join(cursorRoot, "CachedData")
	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	recorder := &recordingHistoryRecorder{}

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		Rules:                  []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		RecycleBinAdapter:      recycle,
		PermanentRemover:       permanent,
		AllowPermanentDeletion: true,
		HistoryRecorder:        recorder,
		OptIn:                  []string{clean.OpportunityCategoryCursorCache},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		DetectRunningApplications: idleCursorDetector(),
	})

	if result.Totals.OptInDeletedCount != 2 {
		t.Fatalf("OptInDeletedCount = %d, want 2; result=%#v", result.Totals.OptInDeletedCount, result)
	}
	if len(recycle.paths) != 0 {
		t.Fatalf("cursor permanent must not use Recycle Bin: %v", recycle.paths)
	}
	found := map[string]bool{}
	for _, path := range permanent.paths {
		found[path] = true
	}
	if !found[cachePath] || !found[cachedDataPath] {
		t.Fatalf("permanent paths = %v, want Cache and CachedData", permanent.paths)
	}
	if len(recorder.items) == 0 {
		t.Fatalf("history items empty, want path-bearing opt-in records")
	}
	for _, item := range recorder.items {
		if item.Path == cachePath || item.Path == cachedDataPath {
			if item.Action != string(clean.DeletionActionDeletePermanently) {
				t.Fatalf("history action = %q", item.Action)
			}
			return
		}
	}
	t.Fatalf("history items = %#v, want Cursor paths", recorder.items)
}

func TestExecuteOptInCursorFreshResolvesNotPreviewPaths(t *testing.T) {
	roamingExecute := t.TempDir()
	cursorRoot := writeCursorRoot(t, roamingExecute, map[string]string{"GPUCache": "execute-root"})
	executePath := filepath.Join(cursorRoot, "GPUCache")
	permanent := &recordingPermanentRemover{}

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		Rules:                  []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		PermanentRemover:       permanent,
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.OpportunityCategoryCursorCache},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roamingExecute,
		},
		DetectRunningApplications: idleCursorDetector(),
	})
	if result.Totals.OptInDeletedCount != 1 {
		t.Fatalf("OptInDeletedCount = %d, want 1", result.Totals.OptInDeletedCount)
	}
	if len(permanent.paths) != 1 || permanent.paths[0] != executePath {
		t.Fatalf("permanent paths = %v, want fresh GPUCache %q", permanent.paths, executePath)
	}
}

func TestExecuteOptInCursorPermanentExcludesRecycleBinCapacity(t *testing.T) {
	roaming := t.TempDir()
	cursorRoot := writeCursorRoot(t, roaming, map[string]string{"Cache": string(make([]byte, 64))})
	cachePath := filepath.Join(cursorRoot, "Cache")
	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	probe := func(path string) (clean.RecycleBinVolumeConfig, error) {
		return clean.RecycleBinVolumeConfig{
			Volume:       filepath.VolumeName(path),
			NukeOnDelete: false,
			MaxCapacity:  1,
			CurrentUsage: 0,
		}, nil
	}
	result := clean.Execute(context.Background(), clean.Options{
		Rules:                   []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		RecycleBinAdapter:       recycle,
		PermanentRemover:        permanent,
		AllowPermanentDeletion:  true,
		RecycleBinCapacityProbe: probe,
		OptIn:                   []string{clean.OpportunityCategoryCursorCache},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		DetectRunningApplications: idleCursorDetector(),
	})
	if len(recycle.paths) != 0 {
		t.Fatalf("recycle adapter called for permanent category: %v", recycle.paths)
	}
	if result.Totals.OptInDeletedCount != 1 || len(permanent.paths) != 1 || permanent.paths[0] != cachePath {
		t.Fatalf("permanent delete failed under tiny capacity: totals=%#v paths=%v", result.Totals, permanent.paths)
	}
}

func TestExecuteOptInCursorDoesNotAuthorizeVSCode(t *testing.T) {
	roaming := t.TempDir()
	writeCursorRoot(t, roaming, map[string]string{"Cache": "cursor"})
	writeVSCodeRoot(t, roaming, map[string]string{"Cache": "vscode"})
	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		Rules:                  []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		PermanentRemover:       permanent,
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.OpportunityCategoryCursorCache},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		DetectRunningApplications: idleBothEditorsDetector(),
	})
	if result.Totals.OptInDeletedCount != 1 {
		t.Fatalf("OptInDeletedCount = %d, want 1 cursor root", result.Totals.OptInDeletedCount)
	}
	for _, path := range permanent.paths {
		relative, err := filepath.Rel(roaming, path)
		if err != nil {
			t.Fatalf("relative permanent path: %v", err)
		}
		applicationRoot := strings.Split(relative, string(filepath.Separator))[0]
		if strings.EqualFold(applicationRoot, "Code") {
			t.Fatalf("cursor opt-in deleted VS Code path: %v", permanent.paths)
		}
	}
}

func writeTraeRoot(t *testing.T, roamingAppData string, rootContents map[string]string) string {
	t.Helper()
	return writeEditorUserDataRoot(t, roamingAppData, "Trae", rootContents)
}

func idleTraeDetector() func(context.Context) []clean.RunningApplicationState {
	return func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{Application: clean.ApplicationTrae, State: clean.RunningApplicationStateIdle},
		}
	}
}

func idleTraeAndVSCodeDetector() func(context.Context) []clean.RunningApplicationState {
	return func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{Application: clean.ApplicationVisualStudioCode, State: clean.RunningApplicationStateIdle},
			{Application: clean.ApplicationTrae, State: clean.RunningApplicationStateIdle},
		}
	}
}

// TestDryRunReportsIdleTraeCacheRootsAsIndependentOpportunities reuses the
// VS Code-family discovery seam: Trae must reclaim exactly the shared
// regenerating-root allowlist (including CachedExtensionVSIXs) under
// %APPDATA%\Trae, with settings/storage/session state never as candidates.
func TestDryRunReportsIdleTraeCacheRootsAsIndependentOpportunities(t *testing.T) {
	roaming := t.TempDir()
	contents := map[string]string{}
	var wantBytes int64
	for _, root := range editorAllowlistedRoots() {
		payload := "trae-" + root
		contents[root] = payload
		wantBytes += int64(len(payload))
	}
	for _, decoy := range []string{
		"User", "workspaceStorage", "globalStorage", "Backups",
		"extensions", "Service Worker", "Local Storage", "Session Storage", "WebStorage",
		"Network", "Cookies", "logs", "Crashpad", "MyCache", "cache-temp",
	} {
		contents[decoy] = "must-not-count"
	}
	traeRoot := writeTraeRoot(t, roaming, contents)
	recorder := &recordingHistoryRecorder{}

	result := clean.DryRun(context.Background(), clean.Options{
		HistoryRecorder: recorder,
		DetailedListDir: filepath.Join(t.TempDir(), "Foal", "history"),
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: t.TempDir(),
		},
		DetectRunningApplications: idleTraeDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	traeOpps := opportunitiesForCategory(result, clean.OpportunityCategoryTraeCache)
	if len(traeOpps) != len(editorAllowlistedRoots()) {
		t.Fatalf("trae opportunities = %#v, want one per allowlisted root", traeOpps)
	}
	seen := make(map[string]bool)
	var total int64
	for _, opportunity := range traeOpps {
		if opportunity.Status != clean.OpportunityStatus || opportunity.Reason != clean.OpportunityReason {
			t.Fatalf("opportunity status/reason = %#v", opportunity)
		}
		if !strings.HasPrefix(opportunity.Path, traeRoot) {
			t.Fatalf("opportunity path %q not under Trae root", opportunity.Path)
		}
		seen[filepath.Base(opportunity.Path)] = true
		total += opportunity.Bytes
	}
	for _, root := range editorAllowlistedRoots() {
		if !seen[root] {
			t.Fatalf("missing allowlisted root %q", root)
		}
	}
	if total != wantBytes || result.Totals.OpportunityObservedBytes != wantBytes || result.Totals.CandidateBytes != 0 {
		t.Fatalf("totals = %#v total=%d want observed-only %d", result.Totals, total, wantBytes)
	}
	if len(opportunitiesForCategory(result, clean.OpportunityCategoryVSCodeCache)) != 0 {
		t.Fatalf("trae roots must not project as vscode_cache: %#v", result.Opportunities)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(encoded)
	if !strings.Contains(jsonText, `"category":"trae_cache"`) {
		t.Fatalf("JSON missing trae_cache: %s", jsonText)
	}
	for _, forbidden := range []string{
		`"category":"vscode_cache"`, "workspaceStorage", "globalStorage", "Backups",
		"Service Worker", "Local Storage", "Cookies", "Crashpad", "MyCache", "cache-temp",
	} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("JSON contains excluded data %q: %s", forbidden, jsonText)
		}
	}

	model := clean.NewPreviewReadModel(result)
	report := clean.RenderPreviewReport(model)
	for _, want := range []string{
		"Developer tools",
		"category: trae_cache",
		"Observed opportunity bytes:",
		"Potential space: 0 bytes",
		"CachedExtensionVSIXs holds downloaded extension packages",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("human report missing %q:\n%s", want, report)
		}
	}

	if len(recorder.sessions) != 1 ||
		recorder.sessions[0].Aggregate.OpportunityCount != len(traeOpps) ||
		recorder.sessions[0].Aggregate.OpportunityObservedBytes != wantBytes ||
		len(recorder.items) != 0 {
		t.Fatalf("history = %#v / %#v, want aggregate-only privacy", recorder.sessions, recorder.items)
	}
	if strings.Contains(recorder.encoded, traeRoot) || strings.Contains(recorder.encoded, "CachedData") {
		t.Fatalf("history leaked editor path: %s", recorder.encoded)
	}
}

func TestDryRunTraeMissingRootIsSilentAbsence(t *testing.T) {
	result := clean.DryRun(context.Background(), clean.Options{
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: t.TempDir(),
		},
		DetectRunningApplications: idleTraeDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryTraeCache {
			t.Fatalf("unexpected trae opportunity: %#v", opportunity)
		}
	}
	if len(result.IncompleteOpportunityInspections) != 0 {
		t.Fatalf("incomplete = %#v, want empty for missing Trae root", result.IncompleteOpportunityInspections)
	}
}

func TestDryRunTraeRunningSkipsWithoutMeasuring(t *testing.T) {
	roaming := t.TempDir()
	writeTraeRoot(t, roaming, map[string]string{"Cache": "cache", "CachedData": "data"})
	result := clean.DryRun(context.Background(), clean.Options{
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationTrae, State: clean.RunningApplicationStateRunning},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryTraeCache {
			t.Fatalf("running Trae produced opportunity: %#v", opportunity)
		}
	}
	if result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("observed bytes = %d, want 0", result.Totals.OpportunityObservedBytes)
	}
	model := clean.NewPreviewReadModel(result)
	report := clean.RenderPreviewReport(model)
	if !strings.Contains(report, "Trae") {
		t.Fatalf("report missing Trae running skip:\n%s", report)
	}
}

func TestDryRunTraeUnknownFailsClosed(t *testing.T) {
	roaming := t.TempDir()
	writeTraeRoot(t, roaming, map[string]string{"Cache": "cache"})
	result := clean.DryRun(context.Background(), clean.Options{
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationTrae, State: clean.RunningApplicationStateUnknown, Message: "snapshot failed"},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryTraeCache {
			t.Fatalf("unknown state produced opportunity: %#v", opportunity)
		}
	}
	found := false
	for _, err := range result.Errors {
		if err.Code == "running_application_detection_unknown" {
			found = true
		}
	}
	if !found {
		t.Fatalf("errors = %#v, want unknown diagnostic", result.Errors)
	}
}

func TestDryRunTraeOptInConvertsRootsToCandidatesWithoutSelectingVSCode(t *testing.T) {
	roaming := t.TempDir()
	writeTraeRoot(t, roaming, map[string]string{
		"Cache":                "cache",
		"CachedExtensionVSIXs": "vsix-pkg",
	})
	writeVSCodeRoot(t, roaming, map[string]string{"Cache": "vscode-only"})
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.OpportunityCategoryTraeCache},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: t.TempDir(),
		},
		DetectRunningApplications: idleTraeAndVSCodeDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if len(opportunitiesForCategory(result, clean.OpportunityCategoryTraeCache)) != 0 {
		t.Fatalf("opted-in trae still listed as opportunity: %#v", result.Opportunities)
	}
	vscodeOpps := opportunitiesForCategory(result, clean.OpportunityCategoryVSCodeCache)
	if len(vscodeOpps) != 1 {
		t.Fatalf("non-opted-in vscode should remain opportunity: %#v", result.Opportunities)
	}
	if len(result.OptInCandidates) != 2 {
		t.Fatalf("opt-in candidates = %#v, want 2 trae roots", result.OptInCandidates)
	}
	for _, candidate := range result.OptInCandidates {
		if candidate.Category != clean.OpportunityCategoryTraeCache {
			t.Fatalf("candidate category = %q, want trae_cache only", candidate.Category)
		}
		if candidate.PlannedAction != string(clean.DeletionActionDeletePermanently) {
			t.Fatalf("planned action = %q, want delete_permanently", candidate.PlannedAction)
		}
	}
	model := clean.NewPreviewReadModel(result)
	foundNotice := false
	for _, notice := range model.Notices {
		if notice.Kind == "opt_in_impact" && strings.Contains(notice.Message, "CachedExtensionVSIXs") {
			foundNotice = true
		}
	}
	if !foundNotice {
		t.Fatalf("notices = %#v, want VSIX impact notice", model.Notices)
	}
}

func TestExecuteWithoutTraeOptInSkipsDetection(t *testing.T) {
	roaming := t.TempDir()
	writeTraeRoot(t, roaming, map[string]string{"Cache": "cache"})
	detectorCalls := 0
	adapter := &recordingRecycleBinAdapter{}
	_ = clean.Execute(context.Background(), clean.Options{
		Rules:             []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		RecycleBinAdapter: adapter,
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			detectorCalls++
			return idleTraeDetector()(context.Background())
		},
	})
	if detectorCalls != 0 {
		t.Fatalf("DetectRunningApplications called %d times without opt-in", detectorCalls)
	}
	if len(adapter.paths) != 0 {
		t.Fatalf("adapter paths = %v", adapter.paths)
	}
}

func TestExecuteOptInTraeCacheCleansWhenIdle(t *testing.T) {
	roaming := t.TempDir()
	traeRoot := writeTraeRoot(t, roaming, map[string]string{
		"Cache":      "cache-data",
		"CachedData": "compiled",
	})
	cachePath := filepath.Join(traeRoot, "Cache")
	cachedDataPath := filepath.Join(traeRoot, "CachedData")
	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	recorder := &recordingHistoryRecorder{}

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		Rules:                  []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		RecycleBinAdapter:      recycle,
		PermanentRemover:       permanent,
		AllowPermanentDeletion: true,
		HistoryRecorder:        recorder,
		OptIn:                  []string{clean.OpportunityCategoryTraeCache},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		DetectRunningApplications: idleTraeDetector(),
	})

	if result.Totals.OptInDeletedCount != 2 {
		t.Fatalf("OptInDeletedCount = %d, want 2; result=%#v", result.Totals.OptInDeletedCount, result)
	}
	if len(recycle.paths) != 0 {
		t.Fatalf("trae permanent must not use Recycle Bin: %v", recycle.paths)
	}
	found := map[string]bool{}
	for _, path := range permanent.paths {
		found[path] = true
	}
	if !found[cachePath] || !found[cachedDataPath] {
		t.Fatalf("permanent paths = %v, want Cache and CachedData", permanent.paths)
	}
	for _, item := range result.Deleted {
		if item.Action != string(clean.DeletionActionDeletePermanently) {
			t.Fatalf("deleted action = %q", item.Action)
		}
	}
	if len(recorder.items) == 0 {
		t.Fatalf("history items empty, want path-bearing opt-in records")
	}
	for _, item := range recorder.items {
		if item.Path == cachePath || item.Path == cachedDataPath {
			if item.Action != string(clean.DeletionActionDeletePermanently) {
				t.Fatalf("history action = %q", item.Action)
			}
			return
		}
	}
	t.Fatalf("history items = %#v, want Trae paths", recorder.items)
}

func TestExecuteOptInTraeFreshResolvesNotPreviewPaths(t *testing.T) {
	roamingExecute := t.TempDir()
	traeRoot := writeTraeRoot(t, roamingExecute, map[string]string{"GPUCache": "execute-root"})
	executePath := filepath.Join(traeRoot, "GPUCache")
	permanent := &recordingPermanentRemover{}

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		Rules:                  []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		PermanentRemover:       permanent,
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.OpportunityCategoryTraeCache},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roamingExecute,
		},
		DetectRunningApplications: idleTraeDetector(),
	})
	if result.Totals.OptInDeletedCount != 1 {
		t.Fatalf("OptInDeletedCount = %d, want 1", result.Totals.OptInDeletedCount)
	}
	if len(permanent.paths) != 1 || permanent.paths[0] != executePath {
		t.Fatalf("permanent paths = %v, want fresh GPUCache %q", permanent.paths, executePath)
	}
}

// TestDryRunIndependentEditorGatesTrae asserts the Trae idle gate is
// independent: a running VS Code must not suppress idle Trae, and a running
// Trae must not suppress idle VS Code.
func TestDryRunIndependentEditorGatesTrae(t *testing.T) {
	roaming := t.TempDir()
	writeVSCodeRoot(t, roaming, map[string]string{"Cache": "vscode-cache"})
	writeTraeRoot(t, roaming, map[string]string{"Cache": "trae-cache"})

	// Running VS Code must not suppress idle Trae.
	result := clean.DryRun(context.Background(), clean.Options{
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		BrowserCacheDiscoveryOptions:     clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: t.TempDir()},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationVisualStudioCode, State: clean.RunningApplicationStateRunning},
				{Application: clean.ApplicationTrae, State: clean.RunningApplicationStateIdle},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if len(opportunitiesForCategory(result, clean.OpportunityCategoryVSCodeCache)) != 0 {
		t.Fatalf("running VS Code still produced opportunities: %#v", result.Opportunities)
	}
	traeOpps := opportunitiesForCategory(result, clean.OpportunityCategoryTraeCache)
	if len(traeOpps) != 1 || filepath.Base(traeOpps[0].Path) != "Cache" {
		t.Fatalf("idle Trae opportunities = %#v, want Cache only", traeOpps)
	}

	// Running Trae must not suppress idle VS Code.
	result = clean.DryRun(context.Background(), clean.Options{
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		BrowserCacheDiscoveryOptions:     clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: t.TempDir()},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationVisualStudioCode, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationTrae, State: clean.RunningApplicationStateRunning},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if len(opportunitiesForCategory(result, clean.OpportunityCategoryTraeCache)) != 0 {
		t.Fatalf("running Trae still produced opportunities: %#v", result.Opportunities)
	}
	vscodeOpps := opportunitiesForCategory(result, clean.OpportunityCategoryVSCodeCache)
	if len(vscodeOpps) != 1 || filepath.Base(vscodeOpps[0].Path) != "Cache" {
		t.Fatalf("idle VS Code opportunities = %#v, want Cache only", vscodeOpps)
	}
}

func TestExecuteOptInTraeDoesNotAuthorizeVSCode(t *testing.T) {
	roaming := t.TempDir()
	writeTraeRoot(t, roaming, map[string]string{"Cache": "trae"})
	writeVSCodeRoot(t, roaming, map[string]string{"Cache": "vscode"})
	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		Rules:                  []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		PermanentRemover:       permanent,
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.OpportunityCategoryTraeCache},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		DetectRunningApplications: idleTraeAndVSCodeDetector(),
	})
	if result.Totals.OptInDeletedCount != 1 {
		t.Fatalf("OptInDeletedCount = %d, want 1 trae root", result.Totals.OptInDeletedCount)
	}
	for _, path := range permanent.paths {
		relative, err := filepath.Rel(roaming, path)
		if err != nil {
			t.Fatalf("relative permanent path: %v", err)
		}
		applicationRoot := strings.Split(relative, string(filepath.Separator))[0]
		if strings.EqualFold(applicationRoot, "Code") {
			t.Fatalf("trae opt-in deleted VS Code path: %v", permanent.paths)
		}
	}
}

func TestDryRunTraeProtectionSuppressesRootBeforeTotals(t *testing.T) {
	roaming := t.TempDir()
	traeRoot := writeTraeRoot(t, roaming, map[string]string{
		"Cache":      "cache",
		"CachedData": "data",
		"GPUCache":   "gpu",
	})
	protected := filepath.Join(traeRoot, "CachedData")
	result := clean.DryRun(context.Background(), clean.Options{
		Validator: pathsafe.NewValidator([]string{protected}),
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		DetectRunningApplications: idleTraeDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryTraeCache && opportunity.Path == protected {
			t.Fatalf("protected root leaked: %#v", opportunity)
		}
	}
	var traeCount int
	var observed int64
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == clean.OpportunityCategoryTraeCache {
			traeCount++
			observed += opportunity.Bytes
		}
	}
	if traeCount != 2 || observed != 8 {
		t.Fatalf("traeCount=%d observed=%d, want 2 siblings totaling 8", traeCount, observed)
	}
}
