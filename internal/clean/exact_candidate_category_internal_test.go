package clean

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

func writeExactCandidateFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func fixtureExactDiscovery(env map[string]string, now time.Time) ExactCandidateDiscoveryOptions {
	return ExactCandidateDiscoveryOptions{
		LookupEnv: func(name string) (string, bool) {
			value, ok := env[name]
			return value, ok
		},
		UserHomeDir: func() (string, error) { return env["USERPROFILE"], nil },
		Now:         now,
	}
}

func fixtureExactCore(t *testing.T, category string, opts Options) categoryCoreResult {
	t.Helper()
	var core categoryCoreResult
	resolveExactCandidateCategory(context.Background(), opts, category, &core)
	return core
}

func TestUnityExactCandidatesRequireDocumentedChildrenAndIdle(t *testing.T) {
	local := t.TempDir()
	root := filepath.Join(local, "Unity", "cache")
	var allowed []string
	for _, parts := range unityPackageCacheChildren {
		child := filepath.Join(append([]string{root}, parts...)...)
		writeExactCandidateFixture(t, filepath.Join(child, "fixture.bin"), []byte("cache"))
		allowed = append(allowed, child)
	}
	writeExactCandidateFixture(t, filepath.Join(root, "git-lfs", "keep.bin"), []byte("keep"))
	writeExactCandidateFixture(t, filepath.Join(root, "upm", "other", "keep.bin"), []byte("keep"))

	detections := 0
	options := Options{
		ExactCandidateDiscoveryOptions: fixtureExactDiscovery(map[string]string{"LOCALAPPDATA": local}, time.Now()),
		DetectRunningApplications: func(context.Context) []RunningApplicationState {
			detections++
			return []RunningApplicationState{{Application: ApplicationUnity, State: RunningApplicationStateIdle}}
		},
	}
	core := fixtureExactCore(t, CategoryUnityPackageCache, options)
	if detections != 2 || len(core.OptInCandidates) != len(allowed) {
		t.Fatalf("detections=%d candidates=%#v", detections, core.OptInCandidates)
	}
	for i, candidate := range core.OptInCandidates {
		if candidate.Path != allowed[i] || candidate.Bytes != 5 || candidate.PlannedAction != string(PlannedActionDeletePermanently) {
			t.Fatalf("candidate[%d]=%#v, want %q", i, candidate, allowed[i])
		}
	}

	options.Validator = pathsafe.NewValidator([]string{allowed[0]})
	core = fixtureExactCore(t, CategoryUnityPackageCache, options)
	if len(core.OptInCandidates) != len(allowed)-1 || len(core.SuppressedProtectionPaths) != 1 {
		t.Fatalf("protected child candidates=%#v suppressed=%#v", core.OptInCandidates, core.SuppressedProtectionPaths)
	}

	options.Validator = pathsafe.Validator{}
	detections = 0
	options.DetectRunningApplications = func(context.Context) []RunningApplicationState {
		detections++
		state := RunningApplicationStateIdle
		if detections == 2 {
			state = RunningApplicationStateRunning
		}
		return []RunningApplicationState{{Application: ApplicationUnity, State: state}}
	}
	core = fixtureExactCore(t, CategoryUnityPackageCache, options)
	if detections != 2 || len(core.OptInCandidates) != 0 || len(core.Skipped) != 1 {
		t.Fatalf("post-measurement running: detections=%d candidates=%#v skipped=%#v", detections, core.OptInCandidates, core.Skipped)
	}
}

func TestEspressifExactCandidatesRequireArchiveAndQuietWindow(t *testing.T) {
	base := t.TempDir()
	tools := filepath.Join(base, "idf-tools")
	profile := filepath.Join(base, "profile")
	drive := filepath.Join(base, "drive")
	dist := filepath.Join(tools, "dist")
	now := time.Now().Truncate(time.Second)
	old := now.Add(-48 * time.Hour)
	archive := filepath.Join(dist, "xtensa-toolchain.tar.xz")
	for _, file := range []string{archive, filepath.Join(dist, "active.zip"), filepath.Join(dist, "future.zip"), filepath.Join(dist, "checksum.txt"), filepath.Join(dist, "unfinished.tmp"), filepath.Join(dist, "nested", "nested.zip")} {
		writeExactCandidateFixture(t, file, []byte("data"))
	}
	for _, file := range []string{archive, filepath.Join(dist, "checksum.txt"), filepath.Join(dist, "unfinished.tmp"), filepath.Join(dist, "nested", "nested.zip")} {
		if err := os.Chtimes(file, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(filepath.Join(dist, "future.zip"), now.Add(time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	writeExactCandidateFixture(t, filepath.Join(tools, "tools", "installed", "version", "keep.bin"), []byte("keep"))
	writeExactCandidateFixture(t, filepath.Join(drive, "Espressif", "dist", "unproven.zip"), []byte("keep"))

	options := Options{ExactCandidateDiscoveryOptions: fixtureExactDiscovery(map[string]string{
		"IDF_TOOLS_PATH": tools, "USERPROFILE": profile, "SystemDrive": drive,
	}, now)}
	core := fixtureExactCore(t, CategoryEspressifToolArchives, options)
	if len(core.OptInCandidates) != 1 || core.OptInCandidates[0].Path != archive {
		t.Fatalf("archive candidates = %#v, want only %q", core.OptInCandidates, archive)
	}
	if len(core.Skipped) != 0 {
		t.Fatalf("unexpected skips: %#v", core.Skipped)
	}

	identity := PermanentIdentityCandidate{Path: archive, Category: CategoryEspressifToolArchives, exactDiscovery: options.ExactCandidateDiscoveryOptions}
	if _, ok := validateExactCandidateIdentity(identity); !ok {
		t.Fatal("unchanged archive lost identity")
	}
	if err := os.Chtimes(archive, now, now); err != nil {
		t.Fatal(err)
	}
	if _, ok := validateExactCandidateIdentity(identity); ok {
		t.Fatal("recently modified archive retained deletion identity")
	}
}

func TestEspressifToolsPathWithoutMarkerYieldsNoRoot(t *testing.T) {
	base := t.TempDir()
	unmarked := filepath.Join(base, "not-idf")
	old := time.Now().Add(-72 * time.Hour)
	archive := filepath.Join(unmarked, "dist", "backup.zip")
	writeExactCandidateFixture(t, archive, []byte("data"))
	if err := os.Chtimes(archive, old, old); err != nil {
		t.Fatal(err)
	}
	options := Options{ExactCandidateDiscoveryOptions: fixtureExactDiscovery(map[string]string{
		"IDF_TOOLS_PATH": unmarked, "USERPROFILE": filepath.Join(base, "profile"),
	}, time.Now())}
	if core := fixtureExactCore(t, CategoryEspressifToolArchives, options); len(core.OptInCandidates) != 0 {
		t.Fatalf("unmarked IDF_TOOLS_PATH yielded candidates %#v", core.OptInCandidates)
	}
	writeExactCandidateFixture(t, filepath.Join(unmarked, "idf-env.json"), []byte("{}"))
	if core := fixtureExactCore(t, CategoryEspressifToolArchives, options); len(core.OptInCandidates) != 1 {
		t.Fatalf("idf-env.json marker candidates = %#v, want the archive", core.OptInCandidates)
	}
}

// vscodeFixture writes one extension folder with a package.json. platform nil
// omits __metadata.targetPlatform.
func vscodeFixture(t *testing.T, root, folder, publisher, name, version string, platform *string) {
	t.Helper()
	metadata := `"installedTimestamp":1`
	if platform != nil {
		metadata += `,"targetPlatform":"` + *platform + `"`
	}
	writeExactCandidateFixture(t, filepath.Join(root, folder, "package.json"),
		[]byte(`{"publisher":"`+publisher+`","name":"`+name+`","version":"`+version+`","__metadata":{`+metadata+`}}`))
}

func TestVSCodeOutdatedExtensionsNeedMarkedVersionOfInstalledExtension(t *testing.T) {
	profile := t.TempDir()
	root := filepath.Join(profile, ".vscode", "extensions")
	win := "win32-x64"
	universal := "universal"
	empty := ""
	vscodeFixture(t, root, "publisher.extension-1.0.0-win32-x64", "publisher", "extension", "1.0.0", &win)
	vscodeFixture(t, root, "publisher.extension-2.0.0-win32-x64", "publisher", "extension", "2.0.0", &win)
	vscodeFixture(t, root, "publisher.universal-1.0.0-universal", "Publisher", "Universal", "1.0.0", &universal)
	vscodeFixture(t, root, "publisher.universal-2.0.0-universal", "Publisher", "Universal", "2.0.0", &universal)
	vscodeFixture(t, root, "publisher.blank-1.0.0", "publisher", "blank", "1.0.0", &empty)
	vscodeFixture(t, root, "publisher.blank-2.0.0", "publisher", "blank", "2.0.0", nil)
	vscodeFixture(t, root, "other.uninstalled-1.0.0", "other", "uninstalled", "1.0.0", nil)
	writeManifest := func(entries string) {
		writeExactCandidateFixture(t, filepath.Join(root, "extensions.json"), []byte("["+entries+"]"))
	}
	writeObsolete := func(keys ...string) {
		body := make([]string, 0, len(keys))
		for _, key := range keys {
			body = append(body, `"`+key+`":true`)
		}
		writeExactCandidateFixture(t, filepath.Join(root, ".obsolete"), []byte("{"+strings.Join(body, ",")+"}"))
	}
	installedManifest := `{"identifier":{"id":"publisher.extension"},"relativeLocation":"publisher.extension-2.0.0-win32-x64"},` +
		`{"identifier":{"id":"publisher.universal"},"relativeLocation":"publisher.universal-2.0.0-universal"},` +
		`{"identifier":{"id":"publisher.blank"},"relativeLocation":"publisher.blank-2.0.0"}`
	writeManifest(installedManifest)
	writeObsolete("publisher.extension-1.0.0-win32-x64", "publisher.universal-1.0.0-universal", "publisher.blank-1.0.0", "other.uninstalled-1.0.0")

	discover := func() []string {
		return discoverVSCodeOutdatedExtensions(context.Background(), exactCandidateDeps{}, root)
	}
	want := []string{filepath.Join(root, "publisher.extension-1.0.0-win32-x64"), filepath.Join(root, "publisher.universal-1.0.0-universal")}
	if got := discover(); !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, want %v (blank platform and uninstalled excluded)", got, want)
	}

	options := Options{
		ExactCandidateDiscoveryOptions: fixtureExactDiscovery(map[string]string{"USERPROFILE": profile}, time.Now()),
		DetectRunningApplications: func(context.Context) []RunningApplicationState {
			return []RunningApplicationState{
				{Application: ApplicationVisualStudioCode, State: RunningApplicationStateIdle},
				{Application: ApplicationVisualStudioCodeInsiders, State: RunningApplicationStateIdle},
			}
		},
	}
	if core := fixtureExactCore(t, CategoryVSCodeOutdatedExtensions, options); len(core.OptInCandidates) != 2 {
		t.Fatalf("resolved candidates = %#v, want 2", core.OptInCandidates)
	}
	identity := PermanentIdentityCandidate{Path: want[0], Category: CategoryVSCodeOutdatedExtensions, exactDiscovery: options.ExactCandidateDiscoveryOptions}
	if _, ok := validateExactCandidateIdentity(identity); !ok {
		t.Fatal("unchanged marked version lost identity")
	}

	// The only referenced version is marked too: VS Code treats the extension
	// as uninstalled and would run its uninstall hook, so nothing qualifies.
	writeObsolete("publisher.extension-1.0.0-win32-x64", "publisher.extension-2.0.0-win32-x64", "publisher.universal-1.0.0-universal")
	if got := discover(); !reflect.DeepEqual(got, want[1:]) {
		t.Fatalf("with the installed version marked, candidates = %v, want %v", got, want[1:])
	}
	if _, ok := validateExactCandidateIdentity(identity); ok {
		t.Fatal("version of an uninstalled extension retained deletion identity")
	}

	// .obsolete keys match VS Code's ExtensionKey exactly (case-sensitive).
	writeObsolete("Publisher.Extension-1.0.0-win32-x64", "publisher.universal-1.0.0-universal")
	if got := discover(); !reflect.DeepEqual(got, want[1:]) {
		t.Fatalf("case-mismatched key candidates = %v, want %v", got, want[1:])
	}

	// The referenced folder is missing: the extension is no longer installed.
	writeObsolete("publisher.extension-1.0.0-win32-x64", "publisher.universal-1.0.0-universal")
	if err := os.RemoveAll(filepath.Join(root, "publisher.universal-2.0.0-universal")); err != nil {
		t.Fatal(err)
	}
	if got := discover(); !reflect.DeepEqual(got, want[:1]) {
		t.Fatalf("with the referenced folder missing, candidates = %v, want %v", got, want[:1])
	}

	// A legacy manifest entry without relativeLocation fails the root closed.
	writeManifest(installedManifest + `,{"identifier":{"id":"legacy.entry"},"location":{"path":"/c:/elsewhere"}}`)
	if got := discover(); len(got) != 0 {
		t.Fatalf("legacy manifest candidates = %v, want none", got)
	}

	// Fully uninstalled: no manifest entry keeps the extension installed.
	writeManifest("")
	if _, ok := validateExactCandidateIdentity(identity); ok {
		t.Fatal("fully uninstalled extension retained deletion identity")
	}
}
