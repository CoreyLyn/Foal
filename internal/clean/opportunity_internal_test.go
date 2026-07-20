package clean

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestBuiltInOpportunityCatalogContainsOnlyApprovedV1Categories(t *testing.T) {
	var got []string
	for _, summary := range CanonicalCleanupCategoryCatalog().Summaries() {
		if summary.Eligibility == CategoryEligibilityOptIn &&
			summary.SelectionPolicy != CategorySelectionPolicyExactOnly &&
			summary.ReportCategory != ReportCategoryBrowsers &&
			summary.ReportCategory != ReportCategoryDeveloperTools &&
			summary.ReportCategory != ReportCategoryApplications {
			got = append(got, summary.Identifier)
		}
	}
	want := []string{
		OpportunityCategoryUserTemp,
		OpportunityCategoryCrashDumps,
		OpportunityCategoryWindowsErrorReporting,
		OpportunityCategoryExplorerThumbnailCache,
		OpportunityCategoryINetCache,
		OpportunityCategoryD3DShaderCache,
		OpportunityCategoryNVIDIADXCache,
		OpportunityCategoryAMDGPUShaderCaches,
		OpportunityCategoryIntelGPUShaderCache,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("built-in opportunity catalog = %#v, want only approved v1 categories %#v", got, want)
	}
}

func TestBuiltInOpportunityDiscoveryNeverInspectsExcludedRoots(t *testing.T) {
	localAppData := `C:\Users\corey\AppData\Local`
	inspected := []string{}
	localAppDataLow := `C:\Users\corey\AppData\LocalLow`
	result := discoverOpportunities(context.Background(), OpportunityDiscoveryOptions{
		TempDir:            `C:\Users\corey\AppData\Local\Temp`,
		LocalAppDataDir:    localAppData,
		LocalAppDataLowDir: localAppDataLow,
	}, opportunityDiscoveryDependencies{
		readDir: func(string) ([]os.DirEntry, error) { return []os.DirEntry{}, nil },
		stat: func(path string) (os.FileInfo, error) {
			inspected = append(inspected, path)
			return nil, fs.ErrNotExist
		},
		walkDir: func(string, fs.WalkDirFunc) error {
			t.Fatal("missing fixed roots must not be inspected")
			return nil
		},
	})

	want := []string{
		filepath.Join(localAppData, "CrashDumps"),
		filepath.Join(localAppData, "Microsoft", "Windows", "WER"),
		// Explorer parent is statted for file-pattern discovery; whole root is never a candidate.
		filepath.Join(localAppData, "Microsoft", "Windows", "Explorer"),
		// INetCache exact allowlisted dirs only (not whole INetCache).
		filepath.Join(localAppData, "Microsoft", "Windows", "INetCache", "IE"),
		filepath.Join(localAppData, "Microsoft", "Windows", "INetCache", "Low", "IE"),
		filepath.Join(localAppData, "D3DSCache"),
		filepath.Join(localAppData, "NVIDIA", "DXCache"),
		filepath.Join(localAppData, "AMD", "DxCache"),
		filepath.Join(localAppData, "AMD", "DxcCache"),
		filepath.Join(localAppData, "AMD", "Dx9Cache"),
		filepath.Join(localAppData, "AMD", "OglCache"),
		filepath.Join(localAppData, "AMD", "VkCache"),
		filepath.Join(localAppDataLow, "AMD", "DxCache"),
		filepath.Join(localAppDataLow, "Intel", "ShaderCache"),
	}
	if !reflect.DeepEqual(inspected, want) {
		t.Fatalf("inspected roots = %#v, want only approved fixed roots %#v", inspected, want)
	}
	if len(result.Opportunities) != 0 || len(result.Incomplete) != 0 {
		t.Fatalf("result = %#v, want excluded and missing roots to contribute nothing", result)
	}
}

func TestIncompleteOpportunityInspectionIsExcludedAndDiscoveryContinues(t *testing.T) {
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	root := `C:\Temp`
	badPath := filepath.Join(root, "blocked")
	validPath := filepath.Join(root, "valid.tmp")
	result := discoverUserTempOpportunities(context.Background(), UserTempDiscoveryOptions{
		TempDir: root,
		Now:     now,
	}, opportunityDiscoveryDependencies{
		readDir: func(string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				fakeDirEntry{name: "blocked", mode: os.ModeDir},
				fakeDirEntry{name: "valid.tmp", size: 5, modified: now.Add(-8 * 24 * time.Hour)},
			}, nil
		},
		walkDir: func(path string, visit fs.WalkDirFunc) error {
			if path == badPath {
				return visit(path, fakeDirEntry{name: "blocked", mode: os.ModeDir}, fs.ErrPermission)
			}
			return visit(path, fakeDirEntry{name: "valid.tmp", size: 5, modified: now.Add(-8 * 24 * time.Hour)}, nil)
		},
	})

	if len(result.Opportunities) != 1 || result.Opportunities[0].Path != validPath {
		t.Fatalf("opportunities = %#v, want only fully inspected entry", result.Opportunities)
	}
	if len(result.Incomplete) != 1 || result.Incomplete[0].Path != badPath {
		t.Fatalf("incomplete = %#v, want blocked entry", result.Incomplete)
	}
	if got := result.Incomplete[0].Reason.Code; got != "permission_denied" {
		t.Fatalf("incomplete reason = %q, want permission_denied", got)
	}
}

func TestCategorizedDiscoveryContinuesAfterUserTempFailure(t *testing.T) {
	localAppData := `C:\Users\corey\AppData\Local`
	localAppDataLow := `C:\Users\corey\AppData\LocalLow`
	// Explorer uses file-pattern discovery under parent; empty ReadDir ⇒ no thumbnail candidates.
	// INetCache is exact IE + Low\IE whole roots.
	expectedRoots := map[string][]string{
		OpportunityCategoryCrashDumps:            {filepath.Join(localAppData, "CrashDumps")},
		OpportunityCategoryWindowsErrorReporting: {filepath.Join(localAppData, "Microsoft", "Windows", "WER")},
		OpportunityCategoryINetCache: {
			filepath.Join(localAppData, "Microsoft", "Windows", "INetCache", "IE"),
			filepath.Join(localAppData, "Microsoft", "Windows", "INetCache", "Low", "IE"),
		},
		OpportunityCategoryD3DShaderCache: {filepath.Join(localAppData, "D3DSCache")},
		OpportunityCategoryNVIDIADXCache:  {filepath.Join(localAppData, "NVIDIA", "DXCache")},
		OpportunityCategoryAMDGPUShaderCaches: {
			filepath.Join(localAppData, "AMD", "DxCache"),
			filepath.Join(localAppData, "AMD", "DxcCache"),
			filepath.Join(localAppData, "AMD", "Dx9Cache"),
			filepath.Join(localAppData, "AMD", "OglCache"),
			filepath.Join(localAppData, "AMD", "VkCache"),
			filepath.Join(localAppDataLow, "AMD", "DxCache"),
		},
		OpportunityCategoryIntelGPUShaderCache: {filepath.Join(localAppDataLow, "Intel", "ShaderCache")},
	}
	explorerParent := filepath.Join(localAppData, "Microsoft", "Windows", "Explorer")
	allowedPaths := map[string]bool{explorerParent: true}
	for _, paths := range expectedRoots {
		for _, p := range paths {
			allowedPaths[p] = true
		}
	}
	result := discoverOpportunities(context.Background(), OpportunityDiscoveryOptions{
		TempDir:            `C:\Users\corey\AppData\Local\Temp`,
		LocalAppDataDir:    localAppData,
		LocalAppDataLowDir: localAppDataLow,
	}, opportunityDiscoveryDependencies{
		readDir: func(path string) ([]os.DirEntry, error) {
			if path == explorerParent {
				return []os.DirEntry{}, nil
			}
			return nil, fs.ErrPermission
		},
		stat: func(path string) (os.FileInfo, error) {
			if allowedPaths[path] {
				return fakeFileInfo{name: filepath.Base(path), mode: os.ModeDir}, nil
			}
			t.Fatalf("unexpected stat path %q", path)
			return nil, fs.ErrNotExist
		},
		walkDir: func(path string, visit fs.WalkDirFunc) error {
			return visit(path, fakeDirEntry{name: filepath.Base(path), mode: os.ModeDir}, nil)
		},
	})

	wantCount := 0
	for _, paths := range expectedRoots {
		wantCount += len(paths)
	}
	if len(result.Opportunities) != wantCount {
		t.Fatalf("opportunities = %#v, want every fixed root despite user temp failure", result.Opportunities)
	}
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == OpportunityCategoryExplorerThumbnailCache {
			t.Fatalf("empty Explorer parent must not emit candidates: %#v", opportunity)
		}
		paths := expectedRoots[opportunity.Category]
		found := false
		for _, p := range paths {
			if opportunity.Path == p {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("opportunity = %#v, want fixed current-user root for category", opportunity)
		}
	}
	if len(result.Incomplete) != 1 ||
		result.Incomplete[0].Category != OpportunityCategoryUserTemp ||
		result.Incomplete[0].Reason.Code != "permission_denied" {
		t.Fatalf("incomplete = %#v, want categorized user temp permission failure", result.Incomplete)
	}
}

func TestExistenceObservedCategorySafetyFailuresAreCategorizedAndExcluded(t *testing.T) {
	categories := []struct {
		category string
		root     string
	}{
		{OpportunityCategoryCrashDumps, `C:\Users\corey\AppData\Local\CrashDumps`},
		{OpportunityCategoryWindowsErrorReporting, `C:\Users\corey\AppData\Local\Microsoft\Windows\WER`},
		// Sample allowlisted candidate paths (file / exact IE root), not whole parents.
		{OpportunityCategoryExplorerThumbnailCache, `C:\Users\corey\AppData\Local\Microsoft\Windows\Explorer\thumbcache_256.db`},
		{OpportunityCategoryINetCache, `C:\Users\corey\AppData\Local\Microsoft\Windows\INetCache\IE`},
		{OpportunityCategoryD3DShaderCache, `C:\Users\corey\AppData\Local\D3DSCache`},
		{OpportunityCategoryNVIDIADXCache, `C:\Users\corey\AppData\Local\NVIDIA\DXCache`},
	}
	tests := []struct {
		name     string
		ctx      context.Context
		stat     func(string) (os.FileInfo, error)
		walkDir  func(string, fs.WalkDirFunc) error
		wantCode string
	}{
		{
			name: "permission failure",
			ctx:  context.Background(),
			stat: func(string) (os.FileInfo, error) { return nil, fs.ErrPermission },
			walkDir: func(string, fs.WalkDirFunc) error {
				t.Fatal("permission failure must not start inspection")
				return nil
			},
			wantCode: "permission_denied",
		},
		{
			name: "reparse point",
			ctx:  context.Background(),
			stat: func(string) (os.FileInfo, error) {
				return fakeFileInfo{name: "CrashDumps", mode: os.ModeDir}, nil
			},
			walkDir: func(path string, visit fs.WalkDirFunc) error {
				return visit(path, fakeDirEntry{name: "CrashDumps", mode: os.ModeSymlink}, nil)
			},
			wantCode: "reparse_point",
		},
		{
			name: "inspection limit",
			ctx:  context.Background(),
			stat: func(string) (os.FileInfo, error) {
				return fakeFileInfo{name: "CrashDumps", mode: os.ModeDir}, nil
			},
			walkDir:  fakeWalkWithDescendants(userTempDescendantLimit+1, time.Now()),
			wantCode: "inspection_limit_exceeded",
		},
		{
			name: "cancellation",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			stat: func(string) (os.FileInfo, error) {
				return fakeFileInfo{name: "CrashDumps", mode: os.ModeDir}, nil
			},
			walkDir: func(path string, visit fs.WalkDirFunc) error {
				return visit(path, fakeDirEntry{name: "CrashDumps", mode: os.ModeDir}, nil)
			},
			wantCode: "context_canceled",
		},
	}
	for _, category := range categories {
		t.Run(category.category, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					result := UserTempDiscoveryResult{}
					appendExistenceObservedOpportunity(test.ctx, &result, category.category, category.root, opportunityDiscoveryDependencies{
						stat:    test.stat,
						walkDir: test.walkDir,
					})

					if len(result.Opportunities) != 0 || len(result.Incomplete) != 1 {
						t.Fatalf("result = %#v, want one incomplete inspection and no partial opportunity", result)
					}
					if result.Incomplete[0].Category != category.category ||
						result.Incomplete[0].Reason.Code != test.wantCode {
						t.Fatalf("incomplete = %#v, want %s/%s", result.Incomplete[0], category.category, test.wantCode)
					}
				})
			}
		})
	}
}

func TestCategorizedDiscoveryContinuesAfterOneFixedCategoryFailure(t *testing.T) {
	localAppData := `C:\Users\corey\AppData\Local`
	localAppDataLow := `C:\Users\corey\AppData\LocalLow`
	d3dRoot := filepath.Join(localAppData, "D3DSCache")
	nvidiaRoot := filepath.Join(localAppData, "NVIDIA", "DXCache")
	// Fixed existence candidates except D3D:
	// CrashDumps + WER + IE + Low\IE + NVIDIA + AMD×6 + Intel = 12.
	// Explorer parent ReadDir is empty ⇒ no thumbnail candidates.
	// D3D is incomplete (permission).
	result := discoverOpportunities(context.Background(), OpportunityDiscoveryOptions{
		TempDir:            `C:\Users\corey\AppData\Local\Temp`,
		LocalAppDataDir:    localAppData,
		LocalAppDataLowDir: localAppDataLow,
	}, opportunityDiscoveryDependencies{
		readDir: func(string) ([]os.DirEntry, error) { return []os.DirEntry{}, nil },
		stat: func(path string) (os.FileInfo, error) {
			if path == d3dRoot {
				return nil, fs.ErrPermission
			}
			return fakeFileInfo{name: filepath.Base(path), mode: os.ModeDir}, nil
		},
		walkDir: func(path string, visit fs.WalkDirFunc) error {
			return visit(path, fakeDirEntry{name: filepath.Base(path), mode: os.ModeDir}, nil)
		},
	})

	if len(result.Opportunities) != 12 {
		t.Fatalf("opportunities = %#v, want unaffected fixed roots (12)", result.Opportunities)
	}
	foundNVIDIA := false
	foundAMD := false
	foundIntel := false
	for _, opportunity := range result.Opportunities {
		if opportunity.Category == OpportunityCategoryD3DShaderCache {
			t.Fatalf("opportunities = %#v, must exclude incomplete D3D category", result.Opportunities)
		}
		if opportunity.Category == OpportunityCategoryNVIDIADXCache && opportunity.Path == nvidiaRoot {
			foundNVIDIA = true
		}
		if opportunity.Category == OpportunityCategoryAMDGPUShaderCaches {
			foundAMD = true
		}
		if opportunity.Category == OpportunityCategoryIntelGPUShaderCache {
			foundIntel = true
		}
	}
	if !foundNVIDIA {
		t.Fatalf("opportunities = %#v, want NVIDIA category after D3D failure", result.Opportunities)
	}
	if !foundAMD || !foundIntel {
		t.Fatalf("opportunities = %#v, want AMD and Intel after D3D failure", result.Opportunities)
	}
	if len(result.Incomplete) != 1 ||
		result.Incomplete[0].Category != OpportunityCategoryD3DShaderCache ||
		result.Incomplete[0].Path != d3dRoot ||
		result.Incomplete[0].Reason.Code != "permission_denied" {
		t.Fatalf("incomplete = %#v, want categorized D3D permission failure", result.Incomplete)
	}
}

func TestUnsafeOpportunityPathsAreIncompleteWithoutInspectionAndDiscoveryContinues(t *testing.T) {
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	root := `C:\Temp`
	validPath := filepath.Join(root, "valid.tmp")
	resolvedPaths := map[string]string{
		"escape":      `C:\Outside\escape`,
		"root-self":   root,
		"nested":      filepath.Join(root, "nested", "child"),
		"unsafe":      `\\server\share\unsafe`,
		"foal-escape": `C:\Outside\foal-escape`,
		"valid.tmp":   validPath,
	}
	inspected := []string{}
	result := discoverUserTempOpportunities(context.Background(), UserTempDiscoveryOptions{
		TempDir: root,
		Now:     now,
	}, opportunityDiscoveryDependencies{
		readDir: func(string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				fakeDirEntry{name: "escape"},
				fakeDirEntry{name: "root-self"},
				fakeDirEntry{name: "nested"},
				fakeDirEntry{name: "unsafe"},
				fakeDirEntry{name: "foal-escape"},
				fakeDirEntry{name: "valid.tmp", size: 5, modified: now.Add(-8 * 24 * time.Hour)},
			}, nil
		},
		joinPath: func(_ string, name string) string {
			return resolvedPaths[name]
		},
		walkDir: func(path string, visit fs.WalkDirFunc) error {
			inspected = append(inspected, path)
			return visit(path, fakeDirEntry{name: filepath.Base(path), size: 5, modified: now.Add(-8 * 24 * time.Hour)}, nil)
		},
	})

	if len(result.Opportunities) != 1 || result.Opportunities[0].Path != validPath {
		t.Fatalf("opportunities = %#v, want only valid direct child", result.Opportunities)
	}
	if len(inspected) != 1 || inspected[0] != validPath {
		t.Fatalf("inspected paths = %#v, want only valid direct child", inspected)
	}
	if len(result.Incomplete) != 5 {
		t.Fatalf("incomplete = %#v, want five unsafe paths", result.Incomplete)
	}
	for _, incomplete := range result.Incomplete {
		if incomplete.Reason.Code != "unsafe_path" || !incomplete.Reason.Recoverable {
			t.Fatalf("incomplete = %#v, want recoverable unsafe_path", incomplete)
		}
	}
}

func TestReparsePointInspectionIsIncomplete(t *testing.T) {
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	root := `C:\Temp`
	result := discoverUserTempOpportunities(context.Background(), UserTempDiscoveryOptions{
		TempDir: root,
		Now:     now,
	}, opportunityDiscoveryDependencies{
		readDir: func(string) ([]os.DirEntry, error) {
			return []os.DirEntry{fakeDirEntry{name: "linked.tmp", mode: os.ModeSymlink}}, nil
		},
		walkDir: func(path string, visit fs.WalkDirFunc) error {
			return visit(path, fakeDirEntry{name: "linked.tmp", mode: os.ModeSymlink}, nil)
		},
	})

	if len(result.Opportunities) != 0 || len(result.Incomplete) != 1 {
		t.Fatalf("result = %#v, want one incomplete entry and no opportunity", result)
	}
	if got := result.Incomplete[0].Reason.Code; got != "reparse_point" {
		t.Fatalf("incomplete reason = %q, want reparse_point", got)
	}
}

func TestOpportunityInspectionDescendantLimit(t *testing.T) {
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name              string
		descendants       int
		wantOpportunities int
		wantIncomplete    int
	}{
		{name: "exactly at limit completes", descendants: userTempDescendantLimit, wantOpportunities: 1},
		{name: "over limit is incomplete", descendants: userTempDescendantLimit + 1, wantIncomplete: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := discoverUserTempOpportunities(context.Background(), UserTempDiscoveryOptions{
				TempDir: `C:\Temp`,
				Now:     now,
			}, opportunityDiscoveryDependencies{
				readDir: func(string) ([]os.DirEntry, error) {
					return []os.DirEntry{fakeDirEntry{name: "large", mode: os.ModeDir, modified: now.Add(-8 * 24 * time.Hour)}}, nil
				},
				walkDir: fakeWalkWithDescendants(test.descendants, now.Add(-8*24*time.Hour)),
			})

			if len(result.Opportunities) != test.wantOpportunities || len(result.Incomplete) != test.wantIncomplete {
				t.Fatalf("opportunities/incomplete = %d/%d, want %d/%d", len(result.Opportunities), len(result.Incomplete), test.wantOpportunities, test.wantIncomplete)
			}
			if test.wantIncomplete == 1 && result.Incomplete[0].Reason.Code != "inspection_limit_exceeded" {
				t.Fatalf("incomplete reason = %q, want inspection_limit_exceeded", result.Incomplete[0].Reason.Code)
			}
		})
	}
}

func TestOpportunityInspectionHonorsCancellation(t *testing.T) {
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	result := discoverUserTempOpportunities(ctx, UserTempDiscoveryOptions{
		TempDir: `C:\Temp`,
		Now:     now,
	}, opportunityDiscoveryDependencies{
		readDir: func(string) ([]os.DirEntry, error) {
			return []os.DirEntry{fakeDirEntry{name: "cancelled", mode: os.ModeDir}}, nil
		},
		walkDir: func(path string, visit fs.WalkDirFunc) error {
			if err := visit(path, fakeDirEntry{name: "cancelled", mode: os.ModeDir, modified: now.Add(-8 * 24 * time.Hour)}, nil); err != nil {
				return err
			}
			cancel()
			return visit(filepath.Join(path, "child.tmp"), fakeDirEntry{name: "child.tmp", size: 1, modified: now.Add(-8 * 24 * time.Hour)}, nil)
		},
	})

	if len(result.Opportunities) != 0 || len(result.Incomplete) != 1 {
		t.Fatalf("result = %#v, want canceled incomplete entry only", result)
	}
	if got := result.Incomplete[0].Reason.Code; got != "context_canceled" {
		t.Fatalf("incomplete reason = %q, want context_canceled", got)
	}
}

func fakeWalkWithDescendants(descendants int, modified time.Time) func(string, fs.WalkDirFunc) error {
	return func(path string, visit fs.WalkDirFunc) error {
		if err := visit(path, fakeDirEntry{name: filepath.Base(path), mode: os.ModeDir, modified: modified}, nil); err != nil {
			return err
		}
		for index := 0; index < descendants; index++ {
			child := filepath.Join(path, "child")
			if err := visit(child, fakeDirEntry{name: "child", modified: modified}, nil); err != nil {
				return err
			}
		}
		return nil
	}
}

type fakeDirEntry struct {
	name     string
	mode     os.FileMode
	size     int64
	modified time.Time
}

func (entry fakeDirEntry) Name() string               { return entry.name }
func (entry fakeDirEntry) IsDir() bool                { return entry.mode.IsDir() }
func (entry fakeDirEntry) Type() os.FileMode          { return entry.mode.Type() }
func (entry fakeDirEntry) Info() (os.FileInfo, error) { return fakeFileInfo(entry), nil }

type fakeFileInfo fakeDirEntry

func (info fakeFileInfo) Name() string       { return info.name }
func (info fakeFileInfo) Size() int64        { return info.size }
func (info fakeFileInfo) Mode() os.FileMode  { return info.mode }
func (info fakeFileInfo) ModTime() time.Time { return info.modified }
func (info fakeFileInfo) IsDir() bool        { return info.mode.IsDir() }
func (info fakeFileInfo) Sys() any           { return nil }
