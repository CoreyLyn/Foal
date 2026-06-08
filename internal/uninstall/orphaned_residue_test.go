package uninstall

import (
	"errors"
	"path/filepath"
	"testing"
)

type fakeResidueRoots map[string][]orphanedResidueEntry

func (f fakeResidueRoots) list(root string) ([]orphanedResidueEntry, error) {
	if entries, ok := f[root]; ok {
		return entries, nil
	}
	return nil, nil
}

func localAppData(segments ...string) string {
	return filepath.Join(append([]string{"LOCALAPPDATA"}, segments...)...)
}

func appData(segments ...string) string {
	return filepath.Join(append([]string{"APPDATA"}, segments...)...)
}

func TestProbeOrphanedResidueIncludesUnmatchedDirectChildren(t *testing.T) {
	fs := fakeResidueRoots{
		"APPDATA":      {{Name: "Abandoned App"}},
		"LOCALAPPDATA": {{Name: "Lonely Tool"}},
	}

	result := probeOrphanedResidue(nil, []string{"APPDATA", "LOCALAPPDATA"}, fs.list)

	if result.Source.Status != "reported" {
		t.Fatalf("source = %#v, want reported", result.Source)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("candidates = %#v, want two", result.Candidates)
	}
	if result.Candidates[0].Path != appData("Abandoned App") || result.Candidates[0].SourceRoot != "APPDATA" {
		t.Fatalf("first candidate = %#v, want APPDATA direct child", result.Candidates[0])
	}
	if result.Candidates[1].Path != localAppData("Lonely Tool") || result.Candidates[1].SourceRoot != "LOCALAPPDATA" {
		t.Fatalf("second candidate = %#v, want LOCALAPPDATA direct child", result.Candidates[1])
	}
}

func TestProbeOrphanedResidueDoesNotRecurseIntoCandidates(t *testing.T) {
	calledNested := false
	list := func(root string) ([]orphanedResidueEntry, error) {
		if root != "APPDATA" {
			calledNested = true
			return nil, nil
		}
		return []orphanedResidueEntry{{Name: "Vendor"}}, nil
	}

	result := probeOrphanedResidue(nil, []string{"APPDATA"}, list)

	if calledNested {
		t.Fatal("listed below a direct child; orphaned residue discovery must not recurse")
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Path != appData("Vendor") {
		t.Fatalf("candidates = %#v, want direct child only", result.Candidates)
	}
}

func TestProbeOrphanedResidueExcludesInstalledApplicationNamesAndPublishers(t *testing.T) {
	fs := fakeResidueRoots{
		"APPDATA": {
			{Name: "Example App"},
			{Name: "Example Co."},
			{Name: "Unmatched"},
		},
	}
	apps := []ApplicationEvidence{{Name: "Example App", Publisher: "Example Co"}}

	result := probeOrphanedResidue(apps, []string{"APPDATA"}, fs.list)

	if len(result.Candidates) != 1 || result.Candidates[0].Path != appData("Unmatched") {
		t.Fatalf("candidates = %#v, want only unmatched directory", result.Candidates)
	}
}

func TestProbeOrphanedResidueExcludesConservativeSharedNames(t *testing.T) {
	fs := fakeResidueRoots{
		"APPDATA": {
			{Name: "Microsoft"},
			{Name: "Google"},
			{Name: "JetBrains"},
			{Name: "Intel"},
			{Name: "Lenovo"},
			{Name: "Small Dead App"},
		},
	}

	result := probeOrphanedResidue(nil, []string{"APPDATA"}, fs.list)

	if len(result.Candidates) != 1 || result.Candidates[0].Path != appData("Small Dead App") {
		t.Fatalf("candidates = %#v, want only non-shared directory", result.Candidates)
	}
}

func TestProbeOrphanedResidueSkipsRiskyDirectoriesAndUnreadableRoots(t *testing.T) {
	fs := fakeResidueRoots{
		"APPDATA": {
			{Name: "Symlinked", Skip: true, Reason: "reparse_point"},
			{Name: "Hidden", Skip: true, Reason: "hidden_or_system"},
			{Name: "Candidate"},
		},
	}
	list := func(root string) ([]orphanedResidueEntry, error) {
		if root == "DENIED" {
			return nil, errors.New("access denied")
		}
		return fs.list(root)
	}

	result := probeOrphanedResidue(nil, []string{"DENIED", "APPDATA"}, list)

	if len(result.Candidates) != 1 || result.Candidates[0].Path != appData("Candidate") {
		t.Fatalf("candidates = %#v, want only safe candidate", result.Candidates)
	}
	if len(result.Skipped) != 3 {
		t.Fatalf("skipped = %#v, want root and two risky directory skips", result.Skipped)
	}
}

func TestProbeOrphanedResidueDistinguishesInspectedEmptyFromNotInspected(t *testing.T) {
	empty := probeOrphanedResidue(nil, []string{"APPDATA"}, fakeResidueRoots{"APPDATA": nil}.list)
	if empty.Source.Status != "reported" || len(empty.Candidates) != 0 || hasSkippedSource(empty.Skipped, orphanedResidueSource) {
		t.Fatalf("empty result = %#v, want inspected reported empty", empty)
	}

	notInspected := probeOrphanedResidue(nil, nil, nil)
	if notInspected.Source.Status != "skipped" || !hasSkippedSource(notInspected.Skipped, orphanedResidueSource) {
		t.Fatalf("not inspected result = %#v, want skipped source", notInspected)
	}
}
