package clean_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/history"
)

var exactCandidateCategories = []string{
	clean.CategoryUnityPackageCache,
	clean.CategoryEspressifToolArchives,
	clean.CategoryVSCodeOutdatedExtensions,
}

// exactCandidateFixture lays out one candidate set per exact-candidate category
// under a test directory, plus siblings that must never become candidates.
type exactCandidateFixture struct {
	env       map[string]string
	now       time.Time
	unity     []string
	archive   string
	extension string
	obsolete  string
}

func writeExactFixtureFile(t *testing.T, path, data string, modified time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if !modified.IsZero() {
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
}

func newExactCandidateFixture(t *testing.T) exactCandidateFixture {
	t.Helper()
	base := t.TempDir()
	now := time.Now().Truncate(time.Second)
	old := now.Add(-72 * time.Hour)
	local := filepath.Join(base, "local")
	profile := filepath.Join(base, "profile")
	tools := filepath.Join(base, "idf")
	fx := exactCandidateFixture{
		env: map[string]string{"LOCALAPPDATA": local, "USERPROFILE": profile, "IDF_TOOLS_PATH": tools},
		now: now,
	}

	unityRoot := filepath.Join(local, "Unity", "cache")
	for _, child := range []string{"npm", "packages"} {
		dir := filepath.Join(unityRoot, child)
		writeExactFixtureFile(t, filepath.Join(dir, "entry.bin"), "unity", time.Time{})
		fx.unity = append(fx.unity, dir)
	}
	writeExactFixtureFile(t, filepath.Join(unityRoot, "git-lfs", "keep.bin"), "keep", time.Time{})

	fx.archive = filepath.Join(tools, "dist", "tool-1.0-win64.zip")
	writeExactFixtureFile(t, fx.archive, "archive", old)
	writeExactFixtureFile(t, filepath.Join(tools, "tools", "tool", "1.0", "tool.exe"), "tool", old)

	extensions := filepath.Join(profile, ".vscode", "extensions")
	fx.obsolete = filepath.Join(extensions, ".obsolete")
	fx.extension = filepath.Join(extensions, "pub.ext-1.0.0")
	writeExactFixtureFile(t, fx.obsolete, `{"pub.ext-1.0.0":true}`, time.Time{})
	writeExactFixtureFile(t, filepath.Join(extensions, "extensions.json"), `[{"identifier":{"id":"pub.ext"},"relativeLocation":"pub.ext-2.0.0"}]`, time.Time{})
	writeExactFixtureFile(t, filepath.Join(fx.extension, "package.json"), `{"publisher":"pub","name":"ext","version":"1.0.0","__metadata":{"installedTimestamp":1}}`, time.Time{})
	writeExactFixtureFile(t, filepath.Join(extensions, "pub.ext-2.0.0", "package.json"), `{"publisher":"pub","name":"ext","version":"2.0.0","__metadata":{"installedTimestamp":2}}`, time.Time{})
	return fx
}

func (fx exactCandidateFixture) discovery() clean.ExactCandidateDiscoveryOptions {
	return clean.ExactCandidateDiscoveryOptions{
		LookupEnv: func(name string) (string, bool) {
			value, ok := fx.env[name]
			return value, ok
		},
		UserHomeDir: func() (string, error) { return fx.env["USERPROFILE"], nil },
		Now:         fx.now,
	}
}

func (fx exactCandidateFixture) candidates() []string {
	paths := append([]string{fx.archive, fx.extension}, fx.unity...)
	sort.Strings(paths)
	return paths
}

func exactCandidateDetector(vscode clean.RunningApplicationStatus) func(context.Context) []clean.RunningApplicationState {
	return func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{Application: clean.ApplicationUnity, State: clean.RunningApplicationStateIdle},
			{Application: clean.ApplicationESPIDFInstallationManager, State: clean.RunningApplicationStateIdle},
			{Application: clean.ApplicationVisualStudioCode, State: vscode},
			{Application: clean.ApplicationVisualStudioCodeInsiders, State: clean.RunningApplicationStateIdle},
		}
	}
}

func exactCandidateExecuteOptions(fx exactCandidateFixture, allowPermanent bool, remover *recordingPermanentRemover, recycle *recordingRecycleBinAdapter) clean.Options {
	return clean.Options{
		AllowPermanentDeletion:         allowPermanent,
		OptIn:                          exactCandidateCategories,
		ExactCandidateDiscoveryOptions: fx.discovery(),
		DetectRunningApplications:      exactCandidateDetector(clean.RunningApplicationStateIdle),
		PermanentRemover:               remover,
		RecycleBinAdapter:              recycle,
		DiscoverOpportunities:          noOpportunities,
		DiscoverReviewSuggestions:      noReviewSuggestions,
		Rules:                          []clean.Rule{{ID: "test", DefaultEnabled: false}},
	}
}

func TestExactCandidateCategoriesDryRunReportsPermanentOptInCandidates(t *testing.T) {
	fx := newExactCandidateFixture(t)
	options := clean.Options{
		OptIn:                          exactCandidateCategories,
		ExactCandidateDiscoveryOptions: fx.discovery(),
		DetectRunningApplications:      exactCandidateDetector(clean.RunningApplicationStateIdle),
		DiscoverOpportunities:          noOpportunities,
		DiscoverUserTempOpportunities:  noUserTempOpportunities,
		DiscoverReviewSuggestions:      noReviewSuggestions,
		Rules:                          []clean.Rule{{ID: "test", DefaultEnabled: false}},
	}
	result := clean.DryRun(context.Background(), options)

	var paths []string
	var bytes int64
	for _, candidate := range result.OptInCandidates {
		if candidate.PlannedAction != string(clean.PlannedActionDeletePermanently) {
			t.Fatalf("candidate %#v, want delete_permanently", candidate)
		}
		paths = append(paths, candidate.Path)
		bytes += candidate.Bytes
	}
	sort.Strings(paths)
	if !reflect.DeepEqual(paths, fx.candidates()) {
		t.Fatalf("candidates = %v, want %v", paths, fx.candidates())
	}
	if result.Totals.OptInReclaimableBytes != bytes || result.Totals.CandidateBytes != 0 {
		t.Fatalf("totals = %#v, want opt-in bytes %d outside Potential space", result.Totals, bytes)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range exactCandidateCategories {
		if !strings.Contains(string(encoded), `"category":"`+category+`"`) {
			t.Fatalf("dry-run JSON missing %s: %s", category, encoded)
		}
	}

	// A running editor removes only that editor's extensions from the preview.
	options.DetectRunningApplications = exactCandidateDetector(clean.RunningApplicationStateRunning)
	result = clean.DryRun(context.Background(), options)
	for _, candidate := range result.OptInCandidates {
		if candidate.Category == clean.CategoryVSCodeOutdatedExtensions {
			t.Fatalf("running VS Code still yields candidate %#v", candidate)
		}
	}
	if len(result.OptInCandidates) != len(fx.candidates())-1 {
		t.Fatalf("candidates = %#v, want all but the extension", result.OptInCandidates)
	}
	running := false
	for _, skipped := range result.Skipped {
		if skipped.Rule == clean.CategoryVSCodeOutdatedExtensions && skipped.Reason.Code == "dev_tool_running" {
			running = true
		}
	}
	if !running {
		t.Fatalf("skipped = %#v, want dev_tool_running for the extensions root", result.Skipped)
	}
}

func TestExactCandidateCategoriesExecuteRequiresPermanentAuthorization(t *testing.T) {
	fx := newExactCandidateFixture(t)
	remover := &recordingPermanentRemover{}
	recycle := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), exactCandidateExecuteOptions(fx, false, remover, recycle))

	if len(remover.paths) != 0 || len(recycle.paths) != 0 || len(result.Deleted) != 0 {
		t.Fatalf("unauthorized run mutated: permanent=%v recycle=%v deleted=%#v", remover.paths, recycle.paths, result.Deleted)
	}
	var skipped []string
	for _, item := range result.Skipped {
		if item.Reason.Code != "permanent_deletion_not_authorized" || item.PlannedAction != string(clean.PlannedActionDeletePermanently) {
			t.Fatalf("skipped item %#v, want unauthorized permanent skip", item)
		}
		skipped = append(skipped, item.Path)
	}
	sort.Strings(skipped)
	if !reflect.DeepEqual(skipped, fx.candidates()) {
		t.Fatalf("skipped = %v, want %v", skipped, fx.candidates())
	}
}

func TestExactCandidateCategoriesExecuteRemovesRevalidatedCandidates(t *testing.T) {
	fx := newExactCandidateFixture(t)
	remover := &recordingPermanentRemover{}
	recycle := &recordingRecycleBinAdapter{}
	recorder := &recordingHistoryRecorder{}
	options := exactCandidateExecuteOptions(fx, true, remover, recycle)
	options.HistoryRecorder = recorder
	options.CommandParameters = history.CommandParameters{Command: "clean", Args: []string{"clean", "--execute", "--allow-permanent"}}
	result := executeCleanWithSafeCapacity(context.Background(), options)

	removed := append([]string(nil), remover.paths...)
	sort.Strings(removed)
	if !reflect.DeepEqual(removed, fx.candidates()) || len(recycle.paths) != 0 {
		t.Fatalf("permanent=%v recycle=%v, want %v and no Recycle Bin", removed, recycle.paths, fx.candidates())
	}
	if len(result.Deleted) != len(fx.candidates()) {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	for _, item := range result.Deleted {
		if item.Action != string(clean.PlannedActionDeletePermanently) || !item.IsOptIn {
			t.Fatalf("deleted item %#v, want permanent opt-in", item)
		}
	}
	recorded := map[string]bool{}
	for _, item := range recorder.items {
		if item.Action == string(clean.PlannedActionDeletePermanently) {
			recorded[item.Rule] = true
		}
	}
	for _, category := range exactCandidateCategories {
		if !recorded[category] {
			t.Fatalf("history items %#v missing %s", recorder.items, category)
		}
	}
}

func TestExactCandidateCategoriesRevalidateImmediatelyBeforeRemoval(t *testing.T) {
	fx := newExactCandidateFixture(t)
	remover := &recordingPermanentRemover{}
	options := exactCandidateExecuteOptions(fx, true, remover, &recordingRecycleBinAdapter{})
	mutated := map[string]bool{}
	options.ProgressReporter = func(progress clean.ExecutionProgress) {
		if progress.Phase != clean.ExecutionPhasePermanentOperations || progress.CompletedCategory != "" || mutated[progress.ActiveCategory] {
			return
		}
		mutated[progress.ActiveCategory] = true
		var err error
		switch progress.ActiveCategory {
		case clean.CategoryEspressifToolArchives:
			// A download rewrote the archive after resolution.
			err = os.Chtimes(fx.archive, fx.now, fx.now)
		case clean.CategoryVSCodeOutdatedExtensions:
			// The editor no longer marks the old version for removal.
			err = os.WriteFile(fx.obsolete, []byte(`{}`), 0o600)
		case clean.CategoryUnityPackageCache:
			// The documented directory was replaced by a same-named file.
			if err = os.RemoveAll(fx.unity[0]); err == nil {
				err = os.WriteFile(fx.unity[0], []byte("x"), 0o600)
			}
		}
		if err != nil {
			t.Errorf("mutate fixture for %s: %v", progress.ActiveCategory, err)
		}
	}
	result := executeCleanWithSafeCapacity(context.Background(), options)

	if len(remover.paths) != 1 || remover.paths[0] != fx.unity[1] {
		t.Fatalf("permanent removals = %v, want only the unchanged %q", remover.paths, fx.unity[1])
	}
	var mismatched []string
	for _, item := range result.Skipped {
		if item.Reason.Code == "identity_mismatch" {
			mismatched = append(mismatched, item.Path)
		}
	}
	sort.Strings(mismatched)
	want := []string{fx.archive, fx.extension, fx.unity[0]}
	sort.Strings(want)
	if !reflect.DeepEqual(mismatched, want) {
		t.Fatalf("identity skips = %v, want %v (all skipped: %#v)", mismatched, want, result.Skipped)
	}
}
