package clean_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// electronNow is the injected clock for electron-updater-residue hermetic tests.
var electronNow = time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)

// electronFixture is a hermetic LOCALAPPDATA root with one valid -updater dir.
type electronFixture struct {
	root        string
	updaterDir  string
	installer   string
	blockmap    string
	pendingDir  string
	pendingExe  string
	pendingInfo string
	pendingMap  string
}

// buildValidElectronFixture creates a fully conforming -updater directory whose
// every allowlisted file is stamped 25 hours old (outside the 24h quiet window).
func buildValidElectronFixture(t *testing.T) electronFixture {
	t.Helper()
	root := t.TempDir()
	updaterDir := filepath.Join(root, "obsidian-updater")
	pendingDir := filepath.Join(updaterDir, "pending")
	if err := os.MkdirAll(pendingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	fx := electronFixture{
		root:        root,
		updaterDir:  updaterDir,
		installer:   filepath.Join(updaterDir, "installer.exe"),
		blockmap:    filepath.Join(updaterDir, "current.blockmap"),
		pendingDir:  pendingDir,
		pendingExe:  filepath.Join(pendingDir, "Obsidian 1.5.exe"),
		pendingInfo: filepath.Join(pendingDir, "update-info.json"),
		pendingMap:  filepath.Join(pendingDir, "current.blockmap"),
	}
	for _, p := range []string{fx.installer, fx.blockmap, fx.pendingExe, fx.pendingInfo, fx.pendingMap} {
		if err := os.WriteFile(p, []byte("payload-"+filepath.Base(p)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stampElectronAll(t, fx, electronNow.Add(-25*time.Hour))
	return fx
}

// stampElectronAll stamps every allowlisted file in the fixture with age,
// skipping files that no longer exist (mutate may have removed them).
func stampElectronAll(t *testing.T, fx electronFixture, mtime time.Time) {
	t.Helper()
	for _, p := range []string{fx.installer, fx.blockmap, fx.pendingExe, fx.pendingInfo, fx.pendingMap} {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			continue
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
}

func electronResolveOptions(fx electronFixture) clean.Options {
	return clean.Options{
		ElectronUpdaterResidueDiscoveryOptions: clean.ElectronUpdaterResidueDiscoveryOptions{
			Root: fx.root,
			Now:  electronNow,
		},
	}
}

func resolveElectron(t *testing.T, opts clean.Options) clean.CategoryResolution {
	t.Helper()
	resolution, err := clean.ResolveCategory(context.Background(), opts, clean.CategoryElectronUpdaterResidue)
	if err != nil {
		t.Fatalf("ResolveCategory error: %v", err)
	}
	return resolution
}

// TestElectronUpdaterResidue_ValidCandidate asserts a fully conforming -updater
// directory yields every allowlisted file as a candidate (update-info.json
// alongside its sibling payload exe).
func TestElectronUpdaterResidue_ValidCandidate(t *testing.T) {
	fx := buildValidElectronFixture(t)
	resolution := resolveElectron(t, electronResolveOptions(fx))

	wantPaths := []string{fx.installer, fx.blockmap, fx.pendingExe, fx.pendingInfo, fx.pendingMap}
	if len(resolution.OptInCandidates) != len(wantPaths) {
		t.Fatalf("candidates = %d, want %d (%#v)", len(resolution.OptInCandidates), len(wantPaths), resolution.OptInCandidates)
	}
	matched := make(map[string]bool)
	for _, c := range resolution.OptInCandidates {
		if c.Category != clean.CategoryElectronUpdaterResidue {
			t.Fatalf("candidate category = %q", c.Category)
		}
		if c.PlannedAction != string(clean.PlannedActionMoveToRecycleBin) {
			t.Fatalf("planned action = %q, want move_to_recycle_bin", c.PlannedAction)
		}
		if c.Bytes <= 0 {
			t.Fatalf("candidate bytes = %d, want > 0", c.Bytes)
		}
		found := false
		for _, want := range wantPaths {
			if strings.EqualFold(c.Path, want) {
				matched[want] = true
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("unexpected candidate path %q", c.Path)
		}
	}
	if len(matched) != len(wantPaths) {
		t.Fatalf("matched %d/%d expected candidate paths", len(matched), len(wantPaths))
	}
	if len(resolution.Skipped) != 0 {
		t.Fatalf("unexpected skips: %#v", resolution.Skipped)
	}
}

// TestElectronUpdaterResidue_RejectedStates asserts each single deviation from
// the valid fixture produces no candidate (silent fail-closed).
func TestElectronUpdaterResidue_RejectedStates(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, fx electronFixture)
	}{
		{"unknown top-level child", func(t *testing.T, fx electronFixture) {
			if err := os.WriteFile(filepath.Join(fx.updaterDir, "stray.dat"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"unknown pending child", func(t *testing.T, fx electronFixture) {
			if err := os.WriteFile(filepath.Join(fx.pendingDir, "stray.dat"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"installer.exe is a directory", func(t *testing.T, fx electronFixture) {
			if err := os.Remove(fx.installer); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(fx.installer, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"pending is a file", func(t *testing.T, fx electronFixture) {
			if err := os.RemoveAll(fx.pendingDir); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fx.pendingDir, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"non-exe non-allowlisted in pending", func(t *testing.T, fx electronFixture) {
			if err := os.WriteFile(filepath.Join(fx.pendingDir, "readme.txt"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := buildValidElectronFixture(t)
			tc.mutate(t, fx)
			// Re-stamp in case mutate touched mtimes; keep all old files stale.
			stampElectronAll(t, fx, electronNow.Add(-25*time.Hour))
			resolution := resolveElectron(t, electronResolveOptions(fx))
			if len(resolution.OptInCandidates) != 0 {
				t.Fatalf("non-conforming directory must yield no candidates, got %#v", resolution.OptInCandidates)
			}
		})
	}
}

// TestElectronUpdaterResidue_SuffixMatch asserts only direct LOCALAPPDATA
// children whose names end with "-updater" (case-insensitive) are matched.
func TestElectronUpdaterResidue_SuffixMatch(t *testing.T) {
	cases := []struct {
		name    string
		dirName string
		match   bool
	}{
		{"standard", "obsidian-updater", true},
		{"capitalized suffix", "Obsidian-Updater", true},
		{"underscore not a match", "obsidian_updater", false},
		{"trailing segment not a match", "obsidian-updater-old", false},
		{"bare updater not a match", "updater", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, tc.dirName)
			if err := os.MkdirAll(filepath.Join(dir, "pending"), 0o700); err != nil {
				t.Fatal(err)
			}
			installer := filepath.Join(dir, "installer.exe")
			if err := os.WriteFile(installer, []byte("p"), 0o600); err != nil {
				t.Fatal(err)
			}
			old := electronNow.Add(-25 * time.Hour)
			if err := os.Chtimes(installer, old, old); err != nil {
				t.Fatal(err)
			}
			resolution := resolveElectron(t, clean.Options{
				ElectronUpdaterResidueDiscoveryOptions: clean.ElectronUpdaterResidueDiscoveryOptions{
					Root: root,
					Now:  electronNow,
				},
			})
			got := len(resolution.OptInCandidates) != 0
			if got != tc.match {
				t.Fatalf("match = %v, want %v (candidates=%#v)", got, tc.match, resolution.OptInCandidates)
			}
		})
	}
}

// TestElectronUpdaterResidue_QuietWindowBoundary asserts exactly-24h is quiet
// (eligible) while just-under-24h skips the whole directory.
func TestElectronUpdaterResidue_QuietWindowBoundary(t *testing.T) {
	t.Run("exactly 24h is eligible", func(t *testing.T) {
		fx := buildValidElectronFixture(t)
		stampElectronAll(t, fx, electronNow.Add(-24*time.Hour))
		resolution := resolveElectron(t, electronResolveOptions(fx))
		if len(resolution.OptInCandidates) != 5 {
			t.Fatalf("exactly-24h must be eligible, candidates = %d (%#v)", len(resolution.OptInCandidates), resolution.OptInCandidates)
		}
		if len(resolution.Skipped) != 0 {
			t.Fatalf("unexpected skips: %#v", resolution.Skipped)
		}
	})
	t.Run("just under 24h skips directory", func(t *testing.T) {
		fx := buildValidElectronFixture(t)
		stampElectronAll(t, fx, electronNow.Add(-23*time.Hour-time.Minute))
		resolution := resolveElectron(t, electronResolveOptions(fx))
		if len(resolution.OptInCandidates) != 0 {
			t.Fatalf("recent directory must yield no candidates, got %#v", resolution.OptInCandidates)
		}
		if len(resolution.Skipped) != 1 || resolution.Skipped[0].Reason.Code != "electron_update_recent" {
			t.Fatalf("expected one electron_update_recent skip, got %#v", resolution.Skipped)
		}
	})
}

// TestElectronUpdaterResidue_PendingOnlyUpdateInfoYieldsNoCandidates asserts a
// pending directory containing only update-info.json yields no candidates.
func TestElectronUpdaterResidue_PendingOnlyUpdateInfoYieldsNoCandidates(t *testing.T) {
	root := t.TempDir()
	updaterDir := filepath.Join(root, "app-updater")
	pendingDir := filepath.Join(updaterDir, "pending")
	if err := os.MkdirAll(pendingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	installer := filepath.Join(updaterDir, "installer.exe")
	info := filepath.Join(pendingDir, "update-info.json")
	if err := os.WriteFile(installer, []byte("p"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(info, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := electronNow.Add(-25 * time.Hour)
	for _, p := range []string{installer, info} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	resolution := resolveElectron(t, clean.Options{
		ElectronUpdaterResidueDiscoveryOptions: clean.ElectronUpdaterResidueDiscoveryOptions{
			Root: root,
			Now:  electronNow,
		},
	})
	// installer.exe is still a candidate; update-info.json (no sibling payload exe) is not.
	if len(resolution.OptInCandidates) != 1 {
		t.Fatalf("candidates = %d, want 1 (installer.exe only; %#v)", len(resolution.OptInCandidates), resolution.OptInCandidates)
	}
	if !strings.EqualFold(resolution.OptInCandidates[0].Path, installer) {
		t.Fatalf("candidate path = %q, want installer.exe", resolution.OptInCandidates[0].Path)
	}
}

// TestElectronUpdaterResidue_PerDirectorySkipIsolation asserts a recent directory
// is skipped while a stale sibling directory still yields candidates.
func TestElectronUpdaterResidue_PerDirectorySkipIsolation(t *testing.T) {
	root := t.TempDir()
	staleDir := filepath.Join(root, "stale-updater")
	freshDir := filepath.Join(root, "fresh-updater")
	for _, dir := range []string{staleDir, freshDir} {
		if err := os.MkdirAll(filepath.Join(dir, "pending"), 0o700); err != nil {
			t.Fatal(err)
		}
		installer := filepath.Join(dir, "installer.exe")
		if err := os.WriteFile(installer, []byte("p"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stale := electronNow.Add(-25 * time.Hour)
	fresh := electronNow.Add(-1 * time.Hour)
	if err := os.Chtimes(filepath.Join(staleDir, "installer.exe"), stale, stale); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(freshDir, "installer.exe"), fresh, fresh); err != nil {
		t.Fatal(err)
	}
	resolution := resolveElectron(t, clean.Options{
		ElectronUpdaterResidueDiscoveryOptions: clean.ElectronUpdaterResidueDiscoveryOptions{
			Root: root,
			Now:  electronNow,
		},
	})
	if len(resolution.OptInCandidates) != 1 {
		t.Fatalf("candidates = %d, want 1 (stale dir only; %#v)", len(resolution.OptInCandidates), resolution.OptInCandidates)
	}
	if !strings.EqualFold(resolution.OptInCandidates[0].Path, filepath.Join(staleDir, "installer.exe")) {
		t.Fatalf("candidate = %q, want stale installer.exe", resolution.OptInCandidates[0].Path)
	}
	if len(resolution.Skipped) != 1 || resolution.Skipped[0].Reason.Code != "electron_update_recent" {
		t.Fatalf("expected one electron_update_recent skip for fresh dir, got %#v", resolution.Skipped)
	}
}

// TestElectronUpdaterResidue_ProtectionSuppression asserts a protected candidate
// file is suppressed rather than produced.
func TestElectronUpdaterResidue_ProtectionSuppression(t *testing.T) {
	fx := buildValidElectronFixture(t)
	opts := electronResolveOptions(fx)
	opts.Validator = pathsafe.NewValidator([]string{fx.installer})
	resolution := resolveElectron(t, opts)
	for _, c := range resolution.OptInCandidates {
		if strings.EqualFold(c.Path, fx.installer) {
			t.Fatalf("protected installer.exe must be suppressed, got %#v", resolution.OptInCandidates)
		}
	}
	if len(resolution.OptInCandidates) == 0 {
		t.Fatalf("non-protected candidates must still be produced, got 0")
	}
	if len(resolution.SuppressedProtectionPaths) == 0 {
		t.Fatal("expected suppressed protection path for protected installer.exe")
	}
}

// TestElectronUpdaterResidue_UpdaterDirProtectionSuppressesAll asserts protecting
// the -updater directory itself suppresses every candidate within it.
func TestElectronUpdaterResidue_UpdaterDirProtectionSuppressesAll(t *testing.T) {
	fx := buildValidElectronFixture(t)
	opts := electronResolveOptions(fx)
	opts.Validator = pathsafe.NewValidator([]string{fx.updaterDir})
	resolution := resolveElectron(t, opts)
	if len(resolution.OptInCandidates) != 0 {
		t.Fatalf("protected updater dir must yield no candidates, got %#v", resolution.OptInCandidates)
	}
	if len(resolution.SuppressedProtectionPaths) == 0 {
		t.Fatal("expected suppressed protection path for protected updater dir")
	}
}

// TestElectronUpdaterResidue_ReparseDirSkipped asserts a reparse-point updater
// directory is never a candidate source.
func TestElectronUpdaterResidue_ReparseDirSkipped(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target-updater")
	if err := os.MkdirAll(filepath.Join(target, "pending"), 0o700); err != nil {
		t.Fatal(err)
	}
	installer := filepath.Join(target, "installer.exe")
	if err := os.WriteFile(installer, []byte("p"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := electronNow.Add(-25 * time.Hour)
	if err := os.Chtimes(installer, old, old); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link-updater")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable on this environment: %v", err)
	}
	resolution := resolveElectron(t, clean.Options{
		ElectronUpdaterResidueDiscoveryOptions: clean.ElectronUpdaterResidueDiscoveryOptions{
			Root: root,
			Now:  electronNow,
		},
	})
	// The symlinked updater dir must not contribute candidates (the real target
	// is also enumerated by its real name, so only one candidate set is expected
	// from the real directory).
	for _, c := range resolution.OptInCandidates {
		if strings.EqualFold(c.Path, filepath.Join(link, "installer.exe")) {
			t.Fatalf("reparse updater dir must not yield candidates, got %#v", resolution.OptInCandidates)
		}
	}
}
