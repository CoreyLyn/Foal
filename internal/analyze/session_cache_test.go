package analyze

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsDurableCachedChild(t *testing.T) {
	cases := []struct {
		name  string
		child BrowseChild
		want  bool
	}{
		{"complete", BrowseChild{State: BrowseStateComplete}, true},
		{"partial", BrowseChild{State: BrowseStatePartial}, true},
		{"skipped", BrowseChild{State: BrowseStateSkipped}, true},
		{"scanning", BrowseChild{State: BrowseStateScanning}, false},
		{
			"hard_limit_incomplete",
			BrowseChild{
				State: BrowseStateIncomplete,
				SkipAggregates: []SkipAggregate{
					{Reason: SkipReasonHardLimit, Count: 1},
				},
			},
			true,
		},
		{
			"cancel_incomplete",
			BrowseChild{
				State: BrowseStateIncomplete,
				SkipAggregates: []SkipAggregate{
					{Reason: SkipReasonCanceled, Count: 1},
				},
			},
			false,
		},
		{
			"incomplete_without_reason",
			BrowseChild{State: BrowseStateIncomplete},
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsDurableCachedChild(tc.child); got != tc.want {
				t.Fatalf("IsDurableCachedChild = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBrowseSessionCachePutIgnoresNavCancelAndScanning(t *testing.T) {
	cache := NewBrowseSessionCache()
	root := `C:\Users\demo`
	cache.Put(root, BrowseChild{
		Name: "done", Path: `C:\Users\demo\done`, State: BrowseStateComplete, Bytes: 10,
	})
	cache.Put(root, BrowseChild{
		Name: "scanning", Path: `C:\Users\demo\scanning`, State: BrowseStateScanning, Bytes: 1,
	})
	cache.Put(root, BrowseChild{
		Name: "canceled", Path: `C:\Users\demo\canceled`, State: BrowseStateIncomplete,
		SkipAggregates: []SkipAggregate{{Reason: SkipReasonCanceled, Count: 1}},
	})
	cache.Put(root, BrowseChild{
		Name: "limited", Path: `C:\Users\demo\limited`, State: BrowseStateIncomplete, Bytes: 5,
		SkipAggregates: []SkipAggregate{{Reason: SkipReasonHardLimit, Count: 1}},
	})

	known := cache.KnownFor(root)
	if len(known) != 2 {
		t.Fatalf("known = %#v, want 2 durable entries", known)
	}
	byName := map[string]BrowseChild{}
	for _, c := range known {
		byName[c.Name] = c
	}
	if byName["done"].Bytes != 10 {
		t.Fatalf("done = %#v", byName["done"])
	}
	if byName["limited"].State != BrowseStateIncomplete {
		t.Fatalf("limited = %#v", byName["limited"])
	}
	if _, ok := byName["scanning"]; ok {
		t.Fatal("scanning must not be cached")
	}
	if _, ok := byName["canceled"]; ok {
		t.Fatal("nav-cancel incomplete must not be cached")
	}
}

func TestBrowseSessionCacheClearLocation(t *testing.T) {
	cache := NewBrowseSessionCache()
	a := `C:\a`
	b := `C:\b`
	cache.Put(a, BrowseChild{Name: "x", Path: `C:\a\x`, State: BrowseStateComplete})
	cache.Put(b, BrowseChild{Name: "y", Path: `C:\b\y`, State: BrowseStateComplete})
	cache.ClearLocation(a)
	if len(cache.KnownFor(a)) != 0 {
		t.Fatal("location a should be cleared")
	}
	if len(cache.KnownFor(b)) != 1 {
		t.Fatal("location b must remain")
	}
}

func TestStreamBrowseReusesKnownDurableChildrenAndMeasuresMissing(t *testing.T) {
	root := t.TempDir()
	// Three directory children.
	for _, name := range []string{"alpha", "beta", "gamma"} {
		dir := filepath.Join(root, name)
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(name+"-data"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Immediate file.
	if err := os.WriteFile(filepath.Join(root, "solo.txt"), []byte("solo"), 0644); err != nil {
		t.Fatal(err)
	}

	alphaPath := filepath.Join(root, "alpha")
	betaPath := filepath.Join(root, "beta")
	// Pretend alpha completed earlier; beta hard-limited; gamma missing.
	known := []BrowseChild{
		{
			Name: "alpha", Path: alphaPath, Kind: BrowseKindDirectory,
			Bytes: 999, FileCount: 1, DirectoryCount: 1,
			State: BrowseStateComplete, Navigable: true,
		},
		{
			Name: "beta", Path: betaPath, Kind: BrowseKindDirectory,
			Bytes: 42, FileCount: 1, DirectoryCount: 1,
			State: BrowseStateIncomplete, Navigable: true,
			SkipAggregates: []SkipAggregate{{Reason: SkipReasonHardLimit, Count: 1}},
		},
	}

	var measured []string
	result := StreamBrowseLocation(context.Background(), root, BrowseOptions{
		KnownChildren: known,
		MeasurementStart: func(path string) {
			measured = append(measured, filepath.Base(path))
		},
	}, nil)
	if !result.OK {
		t.Fatalf("failed: %#v", result.Reason)
	}

	// Only gamma (missing) should be measured; alpha/beta reused; file immediate.
	for _, name := range measured {
		if name == "alpha" || name == "beta" {
			t.Fatalf("durable known child %q must not be re-measured; measured=%v", name, measured)
		}
	}
	foundGamma := false
	for _, name := range measured {
		if name == "gamma" {
			foundGamma = true
		}
	}
	if !foundGamma {
		t.Fatalf("missing gamma must be measured; measured=%v", measured)
	}

	byName := map[string]BrowseChild{}
	for _, c := range result.Children {
		byName[c.Name] = c
	}
	// Reused alpha keeps cached bytes (not re-read from disk).
	if byName["alpha"].Bytes != 999 || byName["alpha"].State != BrowseStateComplete {
		t.Fatalf("alpha reused = %#v", byName["alpha"])
	}
	if byName["beta"].State != BrowseStateIncomplete || byName["beta"].Bytes != 42 {
		t.Fatalf("beta hard-limit reused = %#v", byName["beta"])
	}
	if byName["gamma"].State != BrowseStateComplete {
		t.Fatalf("gamma measured = %#v", byName["gamma"])
	}
	if byName["gamma"].Bytes != int64(len("gamma-data")) {
		t.Fatalf("gamma bytes = %d", byName["gamma"].Bytes)
	}
	if byName["solo.txt"].State != BrowseStateComplete || byName["solo.txt"].Bytes != 4 {
		t.Fatalf("solo = %#v", byName["solo.txt"])
	}
}

func TestStreamBrowseCancelDoesNotProduceDurableHardLimit(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "slow")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		if err := os.WriteFile(filepath.Join(dir, "f"+strconv.Itoa(i)+".txt"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel while measurement is active so Incomplete is cancel-driven.
	started := make(chan struct{})
	var startOnce atomic.Bool
	go func() {
		<-started
		cancel()
	}()

	result := StreamBrowseLocation(ctx, root, BrowseOptions{
		ObservationMinInterval: -1,
		MeasurementStart: func(path string) {
			if startOnce.CompareAndSwap(false, true) {
				close(started)
			}
			// Give cancel a moment to race into the walker.
			time.Sleep(20 * time.Millisecond)
		},
	}, nil)
	if !result.OK {
		t.Fatalf("failed: %#v", result.Reason)
	}

	cache := NewBrowseSessionCache()
	cache.PutAll(root, result.Children)
	// Nav-cancel Incomplete must not enter the session cache.
	for _, c := range result.Children {
		if c.Kind != BrowseKindDirectory {
			continue
		}
		if c.State == BrowseStateComplete {
			// May complete if cancel was late; still fine.
			continue
		}
		if IsDurableCachedChild(c) {
			t.Fatalf("canceled work must not look durable hard-limit: %#v", c)
		}
		if hasSkipReason(c.SkipAggregates, SkipReasonHardLimit) && !hasSkipReason(c.SkipAggregates, SkipReasonCanceled) {
			// Only hard_limit without cancel would be durable; ensure cancel path is present or incomplete not durable.
		}
	}
	if len(cache.KnownFor(root)) > 0 {
		// If something completed before cancel, durable Completes may exist — that is fine.
		for _, c := range cache.KnownFor(root) {
			if c.State == BrowseStateIncomplete && !hasSkipReason(c.SkipAggregates, SkipReasonHardLimit) {
				t.Fatalf("cached incomplete without hard_limit: %#v", c)
			}
			if c.State == BrowseStateIncomplete && hasSkipReason(c.SkipAggregates, SkipReasonCanceled) && !hasSkipReason(c.SkipAggregates, SkipReasonHardLimit) {
				t.Fatalf("nav-cancel incomplete must not be cached: %#v", c)
			}
		}
	}
}

func TestStreamBrowseHardLimitIncompleteIsTerminalOnReuse(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "huge")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if err := os.WriteFile(filepath.Join(dir, "f"+strconv.Itoa(i)+".txt"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	first := BrowseLocation(context.Background(), root, BrowseOptions{DescendantLimit: 5})
	if !first.OK || len(first.Children) != 1 {
		t.Fatalf("first = %#v", first)
	}
	if first.Children[0].State != BrowseStateIncomplete {
		t.Fatalf("state = %q", first.Children[0].State)
	}
	if !hasSkipReason(first.Children[0].SkipAggregates, SkipReasonHardLimit) {
		t.Fatalf("want hard_limit: %#v", first.Children[0].SkipAggregates)
	}

	var measured int
	second := StreamBrowseLocation(context.Background(), root, BrowseOptions{
		DescendantLimit: 5,
		KnownChildren:   first.Children,
		MeasurementStart: func(path string) {
			measured++
		},
	}, nil)
	if !second.OK {
		t.Fatalf("second failed: %#v", second.Reason)
	}
	if measured != 0 {
		t.Fatalf("hard-limit incomplete must not remeasure, measured=%d", measured)
	}
	if second.Children[0].State != BrowseStateIncomplete {
		t.Fatalf("reused state = %q", second.Children[0].State)
	}
}
