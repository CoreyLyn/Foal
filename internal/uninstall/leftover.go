package uninstall

import (
	"path/filepath"
	"strings"
	"unicode"
)

// discoverLeftoverEvidence is the injectable seam for known leftover-location
// discovery. The platform implementation lives in leftover_windows.go /
// leftover_other.go; tests swap this var so they never touch a real profile.
var discoverLeftoverEvidence = discoverPlatformLeftoverEvidence

// LeftoverDiscoveryResult carries the footprint evidence a platform provider
// found, plus the discovery-source status and any recoverable skips.
type LeftoverDiscoveryResult struct {
	Leftovers []LeftoverEvidence
	Source    EvidenceSource
	Skipped   []SkippedReason
}

// rootSpec is a top-level location the footprint provider probes. underProfile
// distinguishes per-user roots (Roaming, Local) from machine-shared roots
// (ProgramData), which are always treated as shared state.
type rootSpec struct {
	path         string
	underProfile bool
}

// dirLister returns the names of the immediate subdirectories of dir. A missing
// directory must return (nil, nil); only real failures return an error. The
// abstraction keeps probeFootprint pure and testable without a real filesystem.
type dirLister func(dir string) ([]string, error)

// probeFootprint surfaces the filesystem footprint of already-discovered (still
// installed) applications by targeted probing — for each application it only
// looks for directories whose normalized name matches that application, and
// never enumerates a root to classify arbitrary directories (that is the
// deferred orphan-residue slice). See docs/adr/0002.
func probeFootprint(apps []ApplicationEvidence, roots []rootSpec, list dirLister) ([]LeftoverEvidence, error) {
	if len(apps) == 0 {
		return nil, nil
	}

	var out []LeftoverEvidence
	seen := map[string]int{}

	emit := func(ev LeftoverEvidence) {
		key := strings.ToLower(ev.Path)
		if idx, ok := seen[key]; ok {
			// Prefer the most specific classification for a shared path: an
			// app-owned footprint outranks a shared-state concern.
			if isAppOwned(ev.Signals) && !isAppOwned(out[idx].Signals) {
				out[idx] = ev
			}
			return
		}
		seen[key] = len(out)
		out = append(out, ev)
	}

	for _, root := range roots {
		topNames, err := list(root.path)
		if err != nil {
			continue // best-effort: an unreadable root contributes nothing
		}
		top := normalizedIndex(topNames)

		for _, app := range apps {
			name := normalizeName(app.Name)
			if name == "" {
				continue
			}
			publisher := normalizeName(app.Publisher)

			// 1. A top-level directory matching the application name is the
			//    application's own footprint.
			if actual, ok := top[name]; ok {
				emit(footprintEvidence(root, app.Name, actual))
				continue
			}

			// 2. A top-level directory matching the publisher is a vendor
			//    directory. Look one level in for the product; otherwise treat
			//    the bare vendor directory as shared publisher state.
			if publisher == "" {
				continue
			}
			vendor, ok := top[publisher]
			if !ok {
				continue
			}
			childNames, cerr := list(filepath.Join(root.path, vendor))
			if cerr == nil {
				if product, ok := normalizedIndex(childNames)[name]; ok {
					emit(footprintEvidence(root, app.Name, vendor, product))
					continue
				}
			}
			emit(sharedEvidence(root, app.Name, vendor))
		}
	}

	return out, nil
}

// footprintEvidence builds evidence for a name-matched directory. Under a user
// profile it carries app-owned signals; a machine-shared root is always shared
// state regardless of the name match.
func footprintEvidence(root rootSpec, app string, segments ...string) LeftoverEvidence {
	path := filepath.Join(append([]string{root.path}, segments...)...)
	if !root.underProfile {
		return LeftoverEvidence{Path: path, App: app, Signals: []string{"shared_program_data"}}
	}
	return LeftoverEvidence{Path: path, App: app, Signals: []string{"app_name_match", "under_user_profile"}}
}

// sharedEvidence builds evidence for a bare vendor directory that holds shared
// publisher state rather than a single application's data.
func sharedEvidence(root rootSpec, app string, segments ...string) LeftoverEvidence {
	path := filepath.Join(append([]string{root.path}, segments...)...)
	return LeftoverEvidence{Path: path, App: app, Signals: []string{"shared_program_data"}}
}

func isAppOwned(signals []string) bool {
	return hasSignal(signals, "app_name_match") && hasSignal(signals, "under_user_profile")
}

// normalizedIndex maps each directory name to its normalized form, keeping the
// first actual name when several collapse to the same key.
func normalizedIndex(names []string) map[string]string {
	index := make(map[string]string, len(names))
	for _, name := range names {
		key := normalizeName(name)
		if key == "" {
			continue
		}
		if _, exists := index[key]; !exists {
			index[key] = name
		}
	}
	return index
}

// normalizeName lower-cases a name and drops every non-alphanumeric rune so that
// whole-name matching ignores spacing and punctuation while still requiring the
// full name to match (e.g. "Google Chrome" never matches a bare "Chrome").
func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
