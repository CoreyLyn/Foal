package uninstall

import (
	"errors"
	"path/filepath"
	"testing"
)

// fakeDirs models a filesystem as a map of directory path to its immediate
// subdirectory names, so probeFootprint can be tested without touching a real
// profile. Keys are built with filepath.Join to match how probeFootprint
// queries nested vendor directories.
type fakeDirs map[string][]string

func (f fakeDirs) list(dir string) ([]string, error) {
	if names, ok := f[dir]; ok {
		return names, nil
	}
	return nil, nil
}

func roaming(segments ...string) string {
	return filepath.Join(append([]string{"ROAMING"}, segments...)...)
}

func programData(segments ...string) string {
	return filepath.Join(append([]string{"PROGRAMDATA"}, segments...)...)
}

func TestProbeFootprintMatchesDisplayNameUnderProfileAsAppOwned(t *testing.T) {
	fs := fakeDirs{"ROAMING": {"Slack", "Other Vendor"}}
	roots := []rootSpec{{path: "ROAMING", underProfile: true}}

	leftovers, err := probeFootprint([]ApplicationEvidence{{Name: "Slack"}}, roots, fs.list)
	if err != nil {
		t.Fatalf("probeFootprint error = %v", err)
	}
	if len(leftovers) != 1 {
		t.Fatalf("leftovers = %#v, want one", leftovers)
	}
	got := leftovers[0]
	if got.Path != roaming("Slack") || got.App != "Slack" {
		t.Fatalf("leftover = %#v, want Roaming\\Slack tied to Slack", got)
	}
	if classifyLeftover(got) != "app_owned" {
		t.Fatalf("classification = %q, want app_owned for %#v", classifyLeftover(got), got)
	}
}

func TestProbeFootprintIgnoresSpacingAndPunctuationButRequiresWholeName(t *testing.T) {
	fs := fakeDirs{"ROAMING": {"google chrome", "Chrome"}}
	roots := []rootSpec{{path: "ROAMING", underProfile: true}}

	// "Google Chrome" matches "google chrome" (normalized), never the bare
	// "Chrome" directory.
	leftovers, _ := probeFootprint([]ApplicationEvidence{{Name: "Google Chrome"}}, roots, fs.list)
	if len(leftovers) != 1 || leftovers[0].Path != roaming("google chrome") {
		t.Fatalf("leftovers = %#v, want only the whole-name match google chrome", leftovers)
	}

	// A generic partial name must not match a longer directory.
	none, _ := probeFootprint([]ApplicationEvidence{{Name: "Chrom"}}, roots, fs.list)
	if len(none) != 0 {
		t.Fatalf("leftovers = %#v, want no partial-name match", none)
	}
}

func TestProbeFootprintMatchesVendorProductNesting(t *testing.T) {
	fs := fakeDirs{
		"ROAMING":            {"JetBrains"},
		roaming("JetBrains"): {"IntelliJ IDEA", "DataGrip"},
	}
	roots := []rootSpec{{path: "ROAMING", underProfile: true}}

	leftovers, _ := probeFootprint([]ApplicationEvidence{{Name: "IntelliJ IDEA", Publisher: "JetBrains"}}, roots, fs.list)
	if len(leftovers) != 1 {
		t.Fatalf("leftovers = %#v, want one nested match", leftovers)
	}
	got := leftovers[0]
	if got.Path != roaming("JetBrains", "IntelliJ IDEA") {
		t.Fatalf("leftover path = %q, want Roaming\\JetBrains\\IntelliJ IDEA", got.Path)
	}
	if classifyLeftover(got) != "app_owned" {
		t.Fatalf("classification = %q, want app_owned", classifyLeftover(got))
	}
}

func TestProbeFootprintBareVendorDirectoryIsSharedState(t *testing.T) {
	fs := fakeDirs{
		"ROAMING":            {"JetBrains"},
		roaming("JetBrains"): {"DataGrip"}, // no IntelliJ product child
	}
	roots := []rootSpec{{path: "ROAMING", underProfile: true}}

	leftovers, _ := probeFootprint([]ApplicationEvidence{{Name: "IntelliJ IDEA", Publisher: "JetBrains"}}, roots, fs.list)
	if len(leftovers) != 1 {
		t.Fatalf("leftovers = %#v, want one bare vendor match", leftovers)
	}
	got := leftovers[0]
	if got.Path != roaming("JetBrains") {
		t.Fatalf("leftover path = %q, want Roaming\\JetBrains", got.Path)
	}
	if classifyLeftover(got) != "shared_state" {
		t.Fatalf("classification = %q, want shared_state for bare vendor dir", classifyLeftover(got))
	}
}

func TestProbeFootprintUnderProgramDataIsAlwaysSharedState(t *testing.T) {
	fs := fakeDirs{"PROGRAMDATA": {"Example App"}}
	roots := []rootSpec{{path: "PROGRAMDATA", underProfile: false}}

	leftovers, _ := probeFootprint([]ApplicationEvidence{{Name: "Example App"}}, roots, fs.list)
	if len(leftovers) != 1 {
		t.Fatalf("leftovers = %#v, want one", leftovers)
	}
	got := leftovers[0]
	if got.Path != programData("Example App") {
		t.Fatalf("leftover path = %q, want ProgramData\\Example App", got.Path)
	}
	if classifyLeftover(got) != "shared_state" {
		t.Fatalf("classification = %q, want shared_state under ProgramData", classifyLeftover(got))
	}
}

func TestProbeFootprintEmitsNothingWithoutAMatch(t *testing.T) {
	fs := fakeDirs{"ROAMING": {"Unrelated"}}
	roots := []rootSpec{{path: "ROAMING", underProfile: true}}

	leftovers, _ := probeFootprint([]ApplicationEvidence{{Name: "Missing App", Publisher: "Nobody"}}, roots, fs.list)
	if len(leftovers) != 0 {
		t.Fatalf("leftovers = %#v, want none", leftovers)
	}
}

func TestProbeFootprintReturnsNothingForNoApplications(t *testing.T) {
	called := false
	list := func(string) ([]string, error) {
		called = true
		return nil, nil
	}
	leftovers, _ := probeFootprint(nil, []rootSpec{{path: "ROAMING", underProfile: true}}, list)
	if len(leftovers) != 0 {
		t.Fatalf("leftovers = %#v, want none", leftovers)
	}
	if called {
		t.Fatal("listed a root with no applications to probe for")
	}
}

func TestProbeFootprintDeduplicatesPathPreferringAppOwned(t *testing.T) {
	// Both applications resolve to ROAMING\Acme: one as an app-owned name match,
	// one as a bare vendor (shared) match. The app-owned classification wins.
	fs := fakeDirs{"ROAMING": {"Acme"}}
	roots := []rootSpec{{path: "ROAMING", underProfile: true}}
	apps := []ApplicationEvidence{
		{Name: "Other", Publisher: "Acme"}, // shared bare-vendor match first
		{Name: "Acme"},                     // app-owned name match second
	}

	leftovers, _ := probeFootprint(apps, roots, fs.list)
	if len(leftovers) != 1 {
		t.Fatalf("leftovers = %#v, want one deduplicated path", leftovers)
	}
	if classifyLeftover(leftovers[0]) != "app_owned" {
		t.Fatalf("classification = %q, want app_owned to win dedup", classifyLeftover(leftovers[0]))
	}
}

func TestProbeFootprintSkipsUnreadableRoot(t *testing.T) {
	roots := []rootSpec{
		{path: "DENIED", underProfile: true},
		{path: "ROAMING", underProfile: true},
	}
	fs := fakeDirs{"ROAMING": {"Slack"}}
	list := func(dir string) ([]string, error) {
		if dir == "DENIED" {
			return nil, errors.New("permission denied")
		}
		return fs.list(dir)
	}

	leftovers, err := probeFootprint([]ApplicationEvidence{{Name: "Slack"}}, roots, list)
	if err != nil {
		t.Fatalf("probeFootprint error = %v, want nil (best-effort)", err)
	}
	if len(leftovers) != 1 || leftovers[0].Path != roaming("Slack") {
		t.Fatalf("leftovers = %#v, want the readable root's match only", leftovers)
	}
}

func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"Google Chrome": "googlechrome",
		"Example Co.":   "exampleco",
		"  Foo-Bar_1 ":  "foobar1",
		"":              "",
		"___":           "",
	}
	for input, want := range cases {
		if got := normalizeName(input); got != want {
			t.Fatalf("normalizeName(%q) = %q, want %q", input, got, want)
		}
	}
}
