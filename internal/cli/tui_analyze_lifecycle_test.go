package cli

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/analyze"
)

func TestAnalyzeBrowseLeaveCancelsCtxAndIgnoresStaleObservations(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})

	var canceled atomic.Bool
	block := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-block:
		default:
			close(block)
		}
	})

	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		if onObservation != nil {
			onObservation(analyze.ChildObservation{
				Name: "scanning", Path: filepath.Join(root, "scanning"), Kind: analyze.BrowseKindDirectory,
				Bytes: 1, State: analyze.BrowseStateScanning, Navigable: true, Terminal: false,
			})
		}
		// Wait until canceled — proves leaveBrowse cancels the context.
		select {
		case <-ctx.Done():
			canceled.Store(true)
			// Emitting after cancel must not land in a newer location (gen discard).
			if onObservation != nil {
				onObservation(analyze.ChildObservation{
					Name: "stale", Path: filepath.Join(root, "stale"), Kind: analyze.BrowseKindDirectory,
					Bytes: 999, State: analyze.BrowseStateComplete, Navigable: true, Terminal: true,
				})
			}
			return analyze.BrowseResult{
				OK:   true,
				Root: root,
				Children: []analyze.BrowseChild{
					{
						Name: "scanning", Path: filepath.Join(root, "scanning"), Kind: analyze.BrowseKindDirectory,
						Bytes: 1, State: analyze.BrowseStateIncomplete, Navigable: true,
						SkipAggregates: []analyze.SkipAggregate{{Reason: analyze.SkipReasonCanceled, Count: 1}},
					},
				},
			}
		case <-time.After(2 * time.Second):
			t.Error("browse was not canceled")
			return analyze.BrowseResult{OK: true, Root: root}
		case <-block:
			return analyze.BrowseResult{OK: true, Root: root}
		}
	})

	model := loadAnalyzeDrive(t)
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("enter must start browse")
	}
	// Start stream and first observation.
	started := cmd()
	next, cmd = model.Update(started)
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("started must wait stream")
	}
	obs := cmd()
	next, _ = model.Update(obs)
	model = next.(rootModel)
	if model.analyze.phase != analyzePhaseBrowse {
		t.Fatalf("phase = %v", model.analyze.phase)
	}

	// Esc to drive entry cancels unfinished work.
	next, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = next.(rootModel)
	if model.analyze.phase != analyzePhaseDrive {
		t.Fatalf("phase after esc = %v, want drive", model.analyze.phase)
	}
	// Give canceled worker time to notice.
	deadline := time.Now().Add(2 * time.Second)
	for !canceled.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !canceled.Load() {
		t.Fatal("leaving location must cancel browse context")
	}
	// Apply a synthetic stale observation with old gen; must not re-enter browse.
	oldGen := model.analyze.gen - 1
	next, _ = model.Update(analyzeBrowseObservationMsg{
		gen: oldGen,
		obs: analyze.ChildObservation{
			Name: "stale", Path: `C:\stale`, Kind: analyze.BrowseKindDirectory,
			Bytes: 999, State: analyze.BrowseStateComplete, Navigable: true, Terminal: true,
		},
	})
	model = next.(rootModel)
	if model.analyze.phase != analyzePhaseDrive {
		t.Fatalf("stale obs must not change phase: %v", model.analyze.phase)
	}
	if model.analyze.browseRoot != "" {
		t.Fatalf("stale obs must not set browseRoot: %q", model.analyze.browseRoot)
	}
}

func TestAnalyzeBrowseSessionCacheResumeAndHardLimitTerminal(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})

	var knownAtCall [][]analyze.BrowseChild
	call := 0
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		knownAtCall = append(knownAtCall, append([]analyze.BrowseChild(nil), opts.KnownChildren...))
		call++
		switch call {
		case 1: // C:\ first visit
			return analyze.BrowseResult{
				OK:   true,
				Root: `C:\`,
				Children: []analyze.BrowseChild{
					{
						Name: "Users", Path: `C:\Users`, Kind: analyze.BrowseKindDirectory,
						Bytes: 100, State: analyze.BrowseStateComplete, Navigable: true,
					},
					{
						Name: "huge", Path: `C:\huge`, Kind: analyze.BrowseKindDirectory,
						Bytes: 50, State: analyze.BrowseStateIncomplete, Navigable: true,
						SkipAggregates: []analyze.SkipAggregate{{Reason: analyze.SkipReasonHardLimit, Count: 1}},
					},
					{
						Name: "pending", Path: `C:\pending`, Kind: analyze.BrowseKindDirectory,
						Bytes: 0, State: analyze.BrowseStateScanning, Navigable: true,
					},
				},
			}
		case 2: // enter Users
			return analyze.BrowseResult{OK: true, Root: `C:\Users`, Children: nil}
		case 3: // return to C:\
			return analyze.BrowseResult{
				OK:   true,
				Root: `C:\`,
				Children: []analyze.BrowseChild{
					{
						Name: "Users", Path: `C:\Users`, Kind: analyze.BrowseKindDirectory,
						Bytes: 100, State: analyze.BrowseStateComplete, Navigable: true,
					},
					{
						Name: "huge", Path: `C:\huge`, Kind: analyze.BrowseKindDirectory,
						Bytes: 50, State: analyze.BrowseStateIncomplete, Navigable: true,
						SkipAggregates: []analyze.SkipAggregate{{Reason: analyze.SkipReasonHardLimit, Count: 1}},
					},
					{
						Name: "pending", Path: `C:\pending`, Kind: analyze.BrowseKindDirectory,
						Bytes: 7, State: analyze.BrowseStateComplete, Navigable: true,
					},
				},
			}
		default:
			return analyze.BrowseResult{OK: true, Root: root}
		}
	})

	model := loadAnalyzeDrive(t)
	model = enterAnalyzeBrowse(t, model)
	// Enter Users (first row after rank).
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("enter Users")
	}
	model = drainAnalyzeBrowse(t, model, cmd)
	// Esc back to C:\
	next, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("esc must reload parent with cache")
	}
	model = drainAnalyzeBrowse(t, model, cmd)

	if len(knownAtCall) < 3 {
		t.Fatalf("browse calls = %d, known snapshots = %d", call, len(knownAtCall))
	}
	// Third call (return) should include durable known children.
	known := knownAtCall[2]
	byName := map[string]analyze.BrowseChild{}
	for _, c := range known {
		byName[c.Name] = c
	}
	if byName["Users"].State != analyze.BrowseStateComplete {
		t.Fatalf("Users should be cached complete: %#v known=%#v", byName["Users"], known)
	}
	if byName["huge"].State != analyze.BrowseStateIncomplete || !hasSkipAgg(byName["huge"], analyze.SkipReasonHardLimit) {
		t.Fatalf("huge hard-limit must be cached terminal: %#v", byName["huge"])
	}
	if c, ok := byName["pending"]; ok && c.State == analyze.BrowseStateScanning {
		t.Fatalf("scanning pending must not be known: %#v", c)
	}
	if !strings.Contains(model.content(), "Users") {
		t.Fatalf("resume missing Users:\n%s", model.content())
	}
}

func TestAnalyzeBrowseRefreshClearsLocationCacheAndRescans(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})
	var knownLens []int
	var call int
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		call++
		knownLens = append(knownLens, len(opts.KnownChildren))
		return analyze.BrowseResult{
			OK:   true,
			Root: root,
			Children: []analyze.BrowseChild{
				{Name: "dir", Path: filepath.Join(root, "dir"), Kind: analyze.BrowseKindDirectory, Bytes: int64(call), State: analyze.BrowseStateComplete, Navigable: true},
			},
		}
	})
	model := loadAnalyzeDrive(t)
	model = enterAnalyzeBrowse(t, model)
	if knownLens[0] != 0 {
		t.Fatalf("first visit known = %d", knownLens[0])
	}
	next, cmd := model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("r must rescan")
	}
	model = drainAnalyzeBrowse(t, model, cmd)
	if len(knownLens) < 2 {
		t.Fatalf("calls = %d", call)
	}
	if knownLens[1] != 0 {
		t.Fatalf("refresh must clear location cache; known=%d", knownLens[1])
	}
	if !strings.Contains(model.content(), "dir") {
		t.Fatalf("refresh missing child:\n%s", model.content())
	}
}

func TestAnalyzeBrowseRefreshOnDriveEntryOnlyReenumeratesVolumes(t *testing.T) {
	var volumeCalls int
	var browseCalls int
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})
	orig := listAnalyzeLocalVolumes
	listAnalyzeLocalVolumes = func() []analyze.LocalVolume {
		volumeCalls++
		return orig()
	}
	t.Cleanup(func() { listAnalyzeLocalVolumes = orig })
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		browseCalls++
		return analyze.BrowseResult{OK: true, Root: root}
	})

	model := loadAnalyzeDrive(t)
	// loadAnalyzeDrive already loaded once; reset counter after load.
	volumeCalls = 0
	browseCalls = 0
	next, cmd := model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("r on drive entry must refresh volumes")
	}
	loaded := cmd()
	next, _ = model.Update(loaded)
	model = next.(rootModel)
	if volumeCalls != 1 {
		t.Fatalf("volume enumeration calls = %d", volumeCalls)
	}
	if browseCalls != 0 {
		t.Fatalf("drive refresh must not browse, browseCalls=%d", browseCalls)
	}
	if model.analyze.phase != analyzePhaseDrive {
		t.Fatalf("phase = %v", model.analyze.phase)
	}
}

func TestAnalyzeBrowseEscHierarchyParentVolumeDriveMenu(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		switch filepath.Clean(root) {
		case `C:\`:
			return analyze.BrowseResult{
				OK:   true,
				Root: `C:\`,
				Children: []analyze.BrowseChild{
					{Name: "Users", Path: `C:\Users`, Kind: analyze.BrowseKindDirectory, Bytes: 1, State: analyze.BrowseStateComplete, Navigable: true},
				},
			}
		case `C:\Users`:
			return analyze.BrowseResult{
				OK:   true,
				Root: `C:\Users`,
				Children: []analyze.BrowseChild{
					{Name: "Public", Path: `C:\Users\Public`, Kind: analyze.BrowseKindDirectory, Bytes: 1, State: analyze.BrowseStateComplete, Navigable: true},
				},
			}
		case `C:\Users\Public`:
			return analyze.BrowseResult{OK: true, Root: `C:\Users\Public`, Children: nil}
		default:
			return analyze.BrowseResult{OK: true, Root: root}
		}
	})

	model := loadAnalyzeDrive(t)
	model = enterAnalyzeBrowse(t, model) // C:\
	// Users
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = drainAnalyzeBrowse(t, next.(rootModel), cmd)
	// Public
	next, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = drainAnalyzeBrowse(t, next.(rootModel), cmd)
	if !strings.Contains(model.content(), `Location: C:\Users\Public`) {
		t.Fatalf("want Public:\n%s", model.content())
	}
	// esc → Users
	next, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = drainAnalyzeBrowse(t, next.(rootModel), cmd)
	if !strings.Contains(model.content(), `Location: C:\Users`) {
		t.Fatalf("want Users:\n%s", model.content())
	}
	// esc → C:\
	next, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = drainAnalyzeBrowse(t, next.(rootModel), cmd)
	if !strings.Contains(model.content(), "Location: C:\\") {
		t.Fatalf("want volume root:\n%s", model.content())
	}
	// esc → drive entry
	next, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = next.(rootModel)
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if _, ok := msg.(analyzeBrowseStartedMsg); ok {
				t.Fatal("esc volume root must not browse")
			}
		}
	}
	if model.analyze.phase != analyzePhaseDrive {
		t.Fatalf("phase = %v want drive", model.analyze.phase)
	}
	// esc → main menu
	next, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = next.(rootModel)
	if model.screen != screenMenu {
		t.Fatalf("screen = %v want menu", model.screen)
	}
}

func TestAnalyzeBrowseNavCancelNotCachedAsHardLimit(t *testing.T) {
	cache := analyze.NewBrowseSessionCache()
	root := `C:\tmp`
	cache.PutAll(root, []analyze.BrowseChild{
		{
			Name: "canceled", Path: `C:\tmp\canceled`, Kind: analyze.BrowseKindDirectory,
			State: analyze.BrowseStateIncomplete,
			SkipAggregates: []analyze.SkipAggregate{{Reason: analyze.SkipReasonCanceled, Count: 1}},
		},
		{
			Name: "limited", Path: `C:\tmp\limited`, Kind: analyze.BrowseKindDirectory,
			State: analyze.BrowseStateIncomplete,
			SkipAggregates: []analyze.SkipAggregate{{Reason: analyze.SkipReasonHardLimit, Count: 1}},
		},
	})
	known := cache.KnownFor(root)
	if len(known) != 1 || known[0].Name != "limited" {
		t.Fatalf("known = %#v, want only hard-limit limited", known)
	}
}

func hasSkipAgg(c analyze.BrowseChild, reason string) bool {
	for _, a := range c.SkipAggregates {
		if a.Reason == reason && a.Count > 0 {
			return true
		}
	}
	return false
}
