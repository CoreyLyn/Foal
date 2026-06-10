package clean

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
