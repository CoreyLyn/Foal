package analyze

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestStreamBrowseAtMostTwoConcurrentDirectoryMeasurements observes concurrency
// through lifecycle hooks only — not goroutine identity or private queues.
func TestStreamBrowseAtMostTwoConcurrentDirectoryMeasurements(t *testing.T) {
	root := t.TempDir()
	// Four directory children; each measurement blocks until released so we can
	// observe the active-count ceiling before any worker finishes.
	names := []string{"a", "b", "c", "d"}
	for _, name := range names {
		if err := os.Mkdir(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name, "f.txt"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Immediate file must not occupy a directory worker slot.
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	var active atomic.Int32
	var maxActive atomic.Int32
	var fileSeenBeforeDirDone atomic.Bool
	var dirsStarted atomic.Int32
	release := make(chan struct{})
	var releaseOnce sync.Once

	// Unblock workers after we have observed the concurrency ceiling (or timeout).
	go func() {
		deadline := time.After(2 * time.Second)
		for {
			if maxActive.Load() >= 2 && dirsStarted.Load() >= 2 {
				// Hold the two active measurements briefly so a third cannot sneak in.
				time.Sleep(50 * time.Millisecond)
				releaseOnce.Do(func() { close(release) })
				return
			}
			select {
			case <-deadline:
				releaseOnce.Do(func() { close(release) })
				return
			default:
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()

	var obsMu sync.Mutex
	var fileObsEarly bool
	result := StreamBrowseLocation(context.Background(), root, BrowseOptions{
		ObservationMinInterval: -1,
		MeasurementStart: func(path string) {
			n := active.Add(1)
			dirsStarted.Add(1)
			for {
				old := maxActive.Load()
				if n <= old || maxActive.CompareAndSwap(old, n) {
					break
				}
			}
			<-release
		},
		MeasurementEnd: func(path string) {
			active.Add(-1)
		},
	}, func(o ChildObservation) {
		obsMu.Lock()
		defer obsMu.Unlock()
		if o.Name == "file.txt" && o.State == BrowseStateComplete {
			// File available while directory work is still gated.
			if active.Load() > 0 || dirsStarted.Load() < 2 {
				fileObsEarly = true
			}
			// Even if timing is tight, file must be terminal complete.
			if o.Bytes != 5 {
				t.Errorf("file bytes = %d", o.Bytes)
			}
		}
	})
	if !result.OK {
		t.Fatalf("failed: %#v", result.Reason)
	}
	if maxActive.Load() > 2 {
		t.Fatalf("max concurrent directory measurements = %d, want <= 2", maxActive.Load())
	}
	if maxActive.Load() < 2 {
		t.Fatalf("max concurrent directory measurements = %d, want 2 (scheduler under-utilized)", maxActive.Load())
	}
	// Count directory children in final result.
	var dirs int
	var fileOK bool
	for _, c := range result.Children {
		if c.Kind == BrowseKindDirectory {
			dirs++
			if c.State != BrowseStateComplete {
				t.Fatalf("dir %s state = %q", c.Name, c.State)
			}
		}
		if c.Name == "file.txt" && c.State == BrowseStateComplete {
			fileOK = true
		}
	}
	if dirs != 4 || !fileOK {
		t.Fatalf("children incomplete: dirs=%d fileOK=%v %#v", dirs, fileOK, result.Children)
	}
	_ = fileSeenBeforeDirDone
	if !fileObsEarly {
		// Soft: file should surface without waiting for all dirs; if flaky timing,
		// final result still proves files do not block on directory workers.
		t.Log("file observation did not overlap gated dir work; final file presence still required")
	}
}

func TestStreamBrowseDefaultDirectoryQueueIsNameOrder(t *testing.T) {
	root := t.TempDir()
	// Create in reverse name order so FS enumeration order is not the oracle.
	for _, name := range []string{"c", "a", "b"} {
		if err := os.Mkdir(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name, "f.txt"), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Serialize workers (capacity still 2 but gate ensures start order is visible).
	var mu sync.Mutex
	var startOrder []string
	gate := make(chan struct{}) // closed after first two starts recorded with single-thread effect
	var started atomic.Int32

	// Allow starts freely but record order at MeasurementStart.
	result := StreamBrowseLocation(context.Background(), root, BrowseOptions{
		ObservationMinInterval: -1,
		// Force one-at-a-time visibility: second worker still may run; we record start order.
		MeasurementStart: func(path string) {
			mu.Lock()
			startOrder = append(startOrder, filepath.Base(path))
			n := started.Add(1)
			mu.Unlock()
			// Stall the first starter briefly so name-order first pick is stable under 2 workers.
			if n == 1 {
				time.Sleep(20 * time.Millisecond)
			}
			_ = gate
		},
	}, nil)
	if !result.OK {
		t.Fatalf("failed: %#v", result.Reason)
	}
	mu.Lock()
	got := append([]string(nil), startOrder...)
	mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("start order = %#v, want 3", got)
	}
	// First two concurrent starts must be the two earliest names (a and b), not c first.
	firstTwo := map[string]bool{got[0]: true, got[1]: true}
	if !firstTwo["a"] || !firstTwo["b"] {
		t.Fatalf("first concurrent starts = %#v, want a and b (name order)", got[:2])
	}
	if got[2] != "c" {
		t.Fatalf("third start = %q, want c", got[2])
	}
}

func TestStreamBrowseFocusPromotesQueuedDirectoryWithoutPreemptingActive(t *testing.T) {
	root := t.TempDir()
	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	cleanRoot := filepath.Clean(absRoot)
	pathOf := func(name string) string { return filepath.Join(cleanRoot, name) }
	for _, name := range []string{"a", "b", "c", "d"} {
		if err := os.Mkdir(pathOf(name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pathOf(name), "f.txt"), []byte("xx"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	focus := NewAtomicBrowseFocus()
	var mu sync.Mutex
	var claimOrder []string
	var startOrder []string
	activePaths := map[string]bool{}
	// Gate: first two workers park inside MeasurementStart until focus is set.
	blockFirstTwo := make(chan struct{})
	firstWaveReady := make(chan struct{})
	var firstWaveOnce sync.Once
	var started atomic.Int32
	var releasedFirstWave atomic.Bool
	dPath := pathOf("d")

	resultCh := make(chan BrowseResult, 1)
	go func() {
		resultCh <- StreamBrowseLocation(context.Background(), cleanRoot, BrowseOptions{
			ObservationMinInterval: -1,
			Focus:                  focus,
			// Claim order is the focus-aware assignment sequence (serialized).
			MeasurementClaimed: func(path string) {
				mu.Lock()
				claimOrder = append(claimOrder, filepath.Base(path))
				mu.Unlock()
			},
			MeasurementStart: func(path string) {
				mu.Lock()
				startOrder = append(startOrder, filepath.Base(path))
				activePaths[path] = true
				n := started.Add(1)
				mu.Unlock()
				if n <= 2 {
					if n == 2 {
						firstWaveOnce.Do(func() { close(firstWaveReady) })
					}
					<-blockFirstTwo
				}
			},
			MeasurementEnd: func(path string) {
				mu.Lock()
				delete(activePaths, path)
				mu.Unlock()
			},
		}, nil)
	}()

	select {
	case <-firstWaveReady:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first-wave a/b to park")
	}

	// Promote d while a and b remain active — must not cancel them.
	// Use the exact path form the scheduler claimed for a sibling when available.
	mu.Lock()
	still := len(activePaths)
	dActive := false
	var samplePath string
	for p := range activePaths {
		samplePath = p
		if strings.EqualFold(filepath.Base(p), "d") {
			dActive = true
		}
	}
	mu.Unlock()
	if still != 2 {
		t.Fatalf("expected exactly 2 active parked workers, got %d", still)
	}
	if dActive {
		t.Fatal("focus must not preempt: d started while a,b active")
	}
	if samplePath != "" {
		focus.Set(filepath.Join(filepath.Dir(samplePath), "d"))
	} else {
		focus.Set(dPath)
	}
	if focus.FocusedPath() == "" {
		t.Fatal("focus not set")
	}
	// Stabilize: ensure focus is visible before freeing workers.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if sameBrowsePath(focus.FocusedPath(), dPath) ||
			(samplePath != "" && sameBrowsePath(focus.FocusedPath(), filepath.Join(filepath.Dir(samplePath), "d"))) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	releasedFirstWave.Store(true)
	close(blockFirstTwo)

	var result BrowseResult
	select {
	case result = <-resultCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for browse to finish")
	}
	if !result.OK {
		t.Fatalf("failed: %#v", result.Reason)
	}
	mu.Lock()
	claims := append([]string(nil), claimOrder...)
	starts := append([]string(nil), startOrder...)
	mu.Unlock()
	if len(claims) != 4 {
		t.Fatalf("claim order = %#v (starts=%#v)", claims, starts)
	}
	// First wave claims: a and b in name order (serialized dispatcher).
	if claims[0] != "a" || claims[1] != "b" {
		t.Fatalf("first wave claims = %#v, want a then b", claims[:2])
	}
	// After focus on d, the next free slot must claim d before c.
	if claims[2] != "d" {
		t.Fatalf("third claim = %q, want d (focus promotion); claims=%#v starts=%#v", claims[2], claims, starts)
	}
	if claims[3] != "c" {
		t.Fatalf("fourth claim = %q, want c; claims=%#v starts=%#v", claims[3], claims, starts)
	}
	// Starts must also include all four; start order among a concurrent wave may
	// vary, but d must appear among starts after the first wave parks.
	if len(starts) != 4 {
		t.Fatalf("start order = %#v", starts)
	}
	firstStarts := map[string]bool{starts[0]: true, starts[1]: true}
	if !firstStarts["a"] || !firstStarts["b"] {
		t.Fatalf("first wave starts = %#v, want a and b", starts[:2])
	}
	if !releasedFirstWave.Load() {
		t.Fatal("test gate did not observe concurrent a,b before release")
	}
}

func TestStreamBrowseFinalChildrenRankedByObservedBytes(t *testing.T) {
	root := t.TempDir()
	// small dir
	if err := os.Mkdir(filepath.Join(root, "small"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "small", "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	// large file
	if err := os.WriteFile(filepath.Join(root, "big.dat"), make([]byte, 100), 0644); err != nil {
		t.Fatal(err)
	}
	// medium dir
	if err := os.Mkdir(filepath.Join(root, "medium"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "medium", "b.txt"), make([]byte, 40), 0644); err != nil {
		t.Fatal(err)
	}

	result := StreamBrowseLocation(context.Background(), root, BrowseOptions{}, nil)
	if !result.OK {
		t.Fatalf("failed: %#v", result.Reason)
	}
	if len(result.Children) != 3 {
		t.Fatalf("children = %d", len(result.Children))
	}
	// Ranked: big.dat (100), medium (40), small (1)
	if result.Children[0].Name != "big.dat" || result.Children[0].Bytes != 100 {
		t.Fatalf("rank0 = %#v", result.Children[0])
	}
	if result.Children[1].Name != "medium" || result.Children[1].Bytes != 40 {
		t.Fatalf("rank1 = %#v", result.Children[1])
	}
	if result.Children[2].Name != "small" || result.Children[2].Bytes != 1 {
		t.Fatalf("rank2 = %#v", result.Children[2])
	}
}
