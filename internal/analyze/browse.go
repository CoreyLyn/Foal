package analyze

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// Injectable filesystem seams for deterministic Partial/Skipped tests.
// Production uses the os package; tests may override and must restore.
var (
	browseReadDir = os.ReadDir
	browseLstat   = os.Lstat
)

// MaxConcurrentDirectoryMeasurements is the fixed concurrency for independent
// direct-child directory trees. Focus promotion never raises this ceiling.
const MaxConcurrentDirectoryMeasurements = 2

// BrowseOptions configures BrowseLocation / StreamBrowseLocation (zero values select defaults).
type BrowseOptions struct {
	// DescendantLimit caps inspected descendants per direct directory child
	// (zero selects default 100_000). Each directory child is measured
	// independently with its own ceiling.
	DescendantLimit int
	// ObservationMinInterval throttles non-terminal Scanning observations.
	// Zero selects DefaultObservationMinInterval. Negative disables throttling.
	// Cadence is never part of correctness; terminals always emit.
	ObservationMinInterval time.Duration
	// Focus optionally prefers a queued directory path for the next free worker
	// slot. Active measurements are never canceled or preempted by focus alone.
	// Nil means pure name-order queueing among remaining work.
	Focus BrowseFocus
	// MeasurementStart is an optional test/observation hook invoked when a
	// directory child measurement begins (after it acquires a worker slot).
	// Must not be used for production control flow. Path is the child root.
	// May block; the scheduler does not hold its lock across this call.
	// Start order among concurrent workers may differ from claim order.
	MeasurementStart func(path string)
	// MeasurementEnd is an optional test/observation hook invoked when a
	// directory child measurement finishes (before releasing the worker slot).
	MeasurementEnd func(path string)
	// MeasurementClaimed is an optional test hook invoked under the scheduler
	// lock when a directory job is assigned to a free slot (focus-aware pick).
	// Claim order is the observable assignment order for focus promotion tests.
	// Must return quickly and must not call back into browse.
	MeasurementClaimed func(path string)
}

// BrowseChild is one direct child of a browse location.
type BrowseChild struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	Kind           string `json:"kind"`
	Bytes          int64  `json:"bytes"`
	FileCount      int64  `json:"file_count"`
	DirectoryCount int64  `json:"directory_count"`
	// Classification is set only for direct children (project_artifact_clue).
	// Recursive measurement never classifies nested artifacts.
	Classification string `json:"classification,omitempty"`
	// State is scanning, complete, partial, incomplete, or skipped.
	State string `json:"state"`
	// SkipReason is set when State is skipped (e.g. reparse_point, permission_denied).
	SkipReason string `json:"skip_reason,omitempty"`
	// SkipAggregates are path-free omission counts under this child (Partial/Incomplete).
	SkipAggregates []SkipAggregate `json:"skip_aggregates,omitempty"`
	// Hidden and System are presentation-only Windows attribute flags.
	Hidden bool `json:"hidden,omitempty"`
	System bool `json:"system,omitempty"`
	// Navigable is true only for ordinary directories (not files, not reparse).
	Navigable bool `json:"navigable"`
}

// BrowseResult is the complete direct-child inventory for one location.
type BrowseResult struct {
	Root      string        `json:"root"`
	Children  []BrowseChild `json:"children"`
	ElapsedMS int64         `json:"elapsed_ms"`
	// Reason is set when the location itself cannot be browsed.
	Reason pathsafe.Reason `json:"reason,omitempty"`
	OK     bool            `json:"ok"`
}

// ObservationHandler receives path-scoped child measurement updates. Handlers must
// not retain unbounded descendant-path lists; ChildObservation is already aggregate.
// Timing of calls is not part of correctness beyond terminal-always-emitted.
type ObservationHandler func(ChildObservation)

// BrowseLocation enumerates every direct child of root after entry and measures
// each directory child recursively with an independent descendant limit.
//
// Contract:
//   - Files expose logical size immediately and do not occupy directory worker slots.
//   - Directories are measured with at most MaxConcurrentDirectoryMeasurements
//     concurrent workers; default queue order is name order; optional Focus
//     promotes a queued path to the next free slot without preempting active work.
//   - Final Children are ranked by latest observed logical bytes descending
//     (name tie-break). Incremental observations are path-scoped; callers re-rank.
//   - Reparse children are visible, not traversed, and not navigable.
//   - Hidden/system children remain visible with presentation-only flags.
//   - Nested project artifacts are not classified during recursive walks.
//   - No sibling locations are prefetched; only root is read.
//   - Read-only: no mutation, elevation, process action, or History write.
//
// Permission/read omissions under a directory → Partial.
// Per-child hard-limit or cancellation stop → Incomplete.
// Direct-child access failure or non-traversed reparse → Skipped.
func BrowseLocation(ctx context.Context, root string, opts BrowseOptions) BrowseResult {
	return StreamBrowseLocation(ctx, root, opts, nil)
}

// StreamBrowseLocation is BrowseLocation plus path-scoped incremental observations.
// onObservation may be nil. Non-terminal Scanning updates are throttled to a
// UI-safe cadence; terminal states always emit. Tests must not depend on exact
// intermediate timing, goroutine identity, or private queue representation.
func StreamBrowseLocation(ctx context.Context, root string, opts BrowseOptions, onObservation ObservationHandler) BrowseResult {
	start := time.Now()
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(root) == "" {
		return BrowseResult{
			OK:     false,
			Reason: pathsafe.Reason{Code: "empty_path", Message: "analyze browse root cannot be empty"},
		}
	}
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return BrowseResult{
			OK:     false,
			Reason: pathsafe.Reason{Code: "invalid_root", Message: "invalid analyze browse root: " + err.Error()},
		}
	}
	if reason, ok := pathsafe.ValidateAnalyzeReadRoot(cleanRoot); !ok {
		return BrowseResult{Root: cleanRoot, OK: false, Reason: reason, ElapsedMS: time.Since(start).Milliseconds()}
	}

	limit := opts.DescendantLimit
	if limit <= 0 {
		limit = defaultDescendantLimit
	}
	minInterval := opts.ObservationMinInterval
	if minInterval == 0 {
		minInterval = DefaultObservationMinInterval
	}
	if minInterval < 0 {
		minInterval = 0
	}

	entries, err := browseReadDir(cleanRoot)
	if err != nil {
		return BrowseResult{
			Root:      cleanRoot,
			OK:        false,
			Reason:    pathsafe.Reason{Code: classifyError(err), Message: err.Error()},
			ElapsedMS: time.Since(start).Milliseconds(),
		}
	}

	// Stable name order for the default directory queue.
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	// Phase 1: classify every direct child. Immediate kinds (file/reparse/skip)
	// emit and complete without worker slots. Directories enqueue for workers.
	childrenByPath := make(map[string]BrowseChild, len(entries))
	var dirJobs []dirJob
	var childOrder []string // discovery order (name-sorted entries)

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			// Cooperative cancel before directory work: return immediate inventory.
			out := make([]BrowseChild, 0, len(childOrder))
			for _, p := range childOrder {
				out = append(out, childrenByPath[p])
			}
			return BrowseResult{
				Root:      cleanRoot,
				Children:  RankBrowseChildren(out),
				OK:        true,
				ElapsedMS: time.Since(start).Milliseconds(),
			}
		default:
		}

		child, needsMeasure := classifyBrowseChild(cleanRoot, entry, onObservation)
		childOrder = append(childOrder, child.Path)
		childrenByPath[child.Path] = child
		if needsMeasure {
			dirJobs = append(dirJobs, dirJob{seed: child})
		}
	}

	// Phase 2: measure directory children with a two-worker, focus-aware scheduler.
	if len(dirJobs) > 0 {
		runDirectoryMeasurements(ctx, dirJobs, limit, minInterval, opts, onObservation, childrenByPath)
	}

	out := make([]BrowseChild, 0, len(childOrder))
	for _, p := range childOrder {
		out = append(out, childrenByPath[p])
	}
	return BrowseResult{
		Root:      cleanRoot,
		Children:  RankBrowseChildren(out),
		OK:        true,
		ElapsedMS: time.Since(start).Milliseconds(),
	}
}

// classifyBrowseChild resolves identity/kind for one direct child. Immediate
// kinds emit terminal observations and return needsMeasure=false. Directory
// children emit an initial Scanning observation and return needsMeasure=true.
func classifyBrowseChild(parent string, entry os.DirEntry, onObservation ObservationHandler) (BrowseChild, bool) {
	name := entry.Name()
	childPath := filepath.Join(parent, name)
	child := BrowseChild{
		Name: name,
		Path: childPath,
	}

	info, err := browseLstat(childPath)
	if err != nil {
		child.Kind = kindFromDirEntry(entry)
		child.State = BrowseStateSkipped
		child.SkipReason = classifyError(err)
		emitObservation(onObservation, child, true)
		return child, false
	}

	attrs := filePresentationAttributes(childPath, info)
	child.Hidden = attrs.Hidden
	child.System = attrs.System

	if attrs.Reparse || info.Mode()&os.ModeSymlink != 0 {
		child.Kind = BrowseKindReparse
		child.State = BrowseStateSkipped
		child.SkipReason = SkipReasonReparsePoint
		child.Navigable = false
		emitObservation(onObservation, child, true)
		return child, false
	}

	if !info.IsDir() {
		child.Kind = BrowseKindFile
		child.Bytes = info.Size()
		child.FileCount = 1
		child.State = BrowseStateComplete
		child.Navigable = false
		emitObservation(onObservation, child, true)
		return child, false
	}

	child.Kind = BrowseKindDirectory
	child.Navigable = true
	child.Classification = childClassification(childPath, BrowseKindDirectory)
	child.State = BrowseStateScanning
	emitObservation(onObservation, child, false)
	return child, true
}

// dirJob is one directory child awaiting independent measurement.
type dirJob struct {
	seed BrowseChild
}

// sameBrowsePath reports whether two canonical browse paths refer to the same
// child. Comparison is case-insensitive via EqualFold after Abs+Clean so focus
// promotion matches seed paths regardless of relative/absolute form.
func sameBrowsePath(a, b string) bool {
	if a == b {
		return true
	}
	if a == "" || b == "" {
		return false
	}
	ca, errA := filepath.Abs(a)
	cb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return strings.EqualFold(filepath.Clean(ca), filepath.Clean(cb))
}

// runDirectoryMeasurements schedules directory jobs with at most
// MaxConcurrentDirectoryMeasurements concurrent measurements. Queue default is
// the order of dirJobs (name-sorted). Focus.FocusedPath, when present in the
// remaining queue, is chosen for the next free slot. Active work is never
// canceled solely due to focus.
//
// A single dispatcher serializes every claim so focus is re-read exactly when a
// free slot is assigned. Workers only measure; they never steal from the queue.
func runDirectoryMeasurements(
	ctx context.Context,
	dirJobs []dirJob,
	limit int,
	minInterval time.Duration,
	opts BrowseOptions,
	onObservation ObservationHandler,
	childrenByPath map[string]BrowseChild,
) {
	if len(dirJobs) == 0 {
		return
	}

	queue := append([]dirJob(nil), dirJobs...)
	var mu sync.Mutex
	var wg sync.WaitGroup
	active := 0
	done := false
	// slotFreed wakes the dispatcher when a worker finishes (or initially).
	slotFreed := make(chan struct{}, MaxConcurrentDirectoryMeasurements)

	// pickNext removes and returns the next job: focused path if queued, else head.
	// Caller must hold mu. Focus is always re-read under the lock at claim time.
	pickNext := func() (dirJob, bool) {
		if len(queue) == 0 {
			return dirJob{}, false
		}
		focusPath := ""
		if opts.Focus != nil {
			focusPath = opts.Focus.FocusedPath()
		}
		if focusPath != "" {
			for i, j := range queue {
				if sameBrowsePath(j.seed.Path, focusPath) {
					job := queue[i]
					queue = append(queue[:i], queue[i+1:]...)
					return job, true
				}
			}
		}
		job := queue[0]
		queue = queue[1:]
		return job, true
	}

	// notifyDispatcher signals that capacity may be available.
	notifyDispatcher := func() {
		select {
		case slotFreed <- struct{}{}:
		default:
		}
	}

	// startWorker runs one claimed job. Must not hold mu.
	startWorker := func(j dirJob) {
		wg.Add(1)
		go func(job dirJob) {
			defer wg.Done()
			if opts.MeasurementStart != nil {
				opts.MeasurementStart(job.seed.Path)
			}
			child := measureBrowseDirectory(ctx, job.seed, limit, minInterval, onObservation)
			if opts.MeasurementEnd != nil {
				opts.MeasurementEnd(job.seed.Path)
			}

			mu.Lock()
			childrenByPath[child.Path] = child
			active--
			remaining := len(queue)
			mu.Unlock()

			if remaining > 0 {
				notifyDispatcher()
			}
		}(j)
	}

	// Dispatch until queue empty and no active workers.
	// Initial kicks fill up to the concurrency ceiling; further kicks come from
	// worker completion. Focus is consulted on every claim.
	for {
		mu.Lock()
		// Fill free slots while work remains.
		for active < MaxConcurrentDirectoryMeasurements && len(queue) > 0 {
			job, ok := pickNext()
			if !ok {
				break
			}
			active++
			if opts.MeasurementClaimed != nil {
				opts.MeasurementClaimed(job.seed.Path)
			}
			mu.Unlock()
			startWorker(job)
			mu.Lock()
		}
		idle := active == 0 && len(queue) == 0
		if idle {
			done = true
		}
		mu.Unlock()

		if done {
			break
		}

		// Wait for a worker to free a slot, or for all work to finish.
		// If active workers exist but queue is empty, wait for them to finish
		// via wg; if queue still has work, wait on slotFreed.
		mu.Lock()
		needWait := active > 0 || len(queue) > 0
		queueEmpty := len(queue) == 0
		mu.Unlock()
		if !needWait {
			break
		}
		if queueEmpty {
			// Only active work left; wait for all workers.
			wg.Wait()
			continue
		}
		// Work queued but slots full (or racing): wait for a free signal.
		// Also handle the case where slots free without a signal (use short poll
		// via default after checking active again).
		select {
		case <-slotFreed:
		case <-ctx.Done():
			// Still drain workers; cancel is handled inside measure.
			wg.Wait()
			return
		case <-time.After(10 * time.Millisecond):
			// Poll: a worker may have finished between our check and select.
		}
	}
	wg.Wait()
}

// measureBrowseDirectory runs independent recursive measurement for one
// directory child that already emitted its initial Scanning observation.
func measureBrowseDirectory(
	ctx context.Context,
	child BrowseChild,
	limit int,
	minInterval time.Duration,
	onObservation ObservationHandler,
) BrowseChild {
	throttle := newObservationThrottle(minInterval)
	if minInterval > 0 {
		throttle.lastEmit = time.Now()
		if throttle.now != nil {
			throttle.lastEmit = throttle.now()
		}
	}

	report := func(obs ChildObservation) {
		if onObservation == nil {
			return
		}
		if !throttle.allow(obs.Terminal) {
			return
		}
		onObservation(obs)
	}

	outcome := measureDirectoryTree(ctx, child.Path, limit, func(progress measureProgress) {
		obs := ChildObservation{
			Name:           child.Name,
			Path:           child.Path,
			Kind:           child.Kind,
			Bytes:          progress.Totals.Bytes,
			FileCount:      progress.Totals.FileCount,
			DirectoryCount: progress.Totals.DirectoryCount,
			Classification: child.Classification,
			State:          BrowseStateScanning,
			SkipAggregates: aggregatesFromMap(progress.SkipCounts),
			Hidden:         child.Hidden,
			System:         child.System,
			Navigable:      child.Navigable,
			Terminal:       false,
		}
		report(obs)
	})

	child.Bytes = outcome.Totals.Bytes
	child.FileCount = outcome.Totals.FileCount
	child.DirectoryCount = outcome.Totals.DirectoryCount
	child.SkipAggregates = aggregatesFromMap(outcome.SkipCounts)
	child.State = outcome.State
	if outcome.State == BrowseStateSkipped {
		child.SkipReason = outcome.DirectSkipReason
	}
	emitObservation(onObservation, child, true)
	return child
}

func emitObservation(on ObservationHandler, child BrowseChild, terminal bool) {
	if on == nil {
		return
	}
	on(ChildObservation{
		Name:           child.Name,
		Path:           child.Path,
		Kind:           child.Kind,
		Bytes:          child.Bytes,
		FileCount:      child.FileCount,
		DirectoryCount: child.DirectoryCount,
		Classification: child.Classification,
		State:          child.State,
		SkipReason:     child.SkipReason,
		SkipAggregates: append([]SkipAggregate(nil), child.SkipAggregates...),
		Hidden:         child.Hidden,
		System:         child.System,
		Navigable:      child.Navigable,
		Terminal:       terminal || IsTerminalBrowseState(child.State),
	})
}

func kindFromDirEntry(entry os.DirEntry) string {
	if entry.Type()&os.ModeSymlink != 0 {
		return BrowseKindReparse
	}
	if entry.IsDir() {
		return BrowseKindDirectory
	}
	return BrowseKindFile
}

// measureProgress is an incremental recursive-measurement snapshot.
type measureProgress struct {
	Totals     Totals
	SkipCounts map[string]int64
}

// measureOutcome is the terminal recursive-measurement result for one directory child.
type measureOutcome struct {
	Totals           Totals
	SkipCounts       map[string]int64
	State            string
	DirectSkipReason string
}

// measureDirectoryTree measures path as an independent tree with its own
// descendant ceiling. It does not classify nested project artifacts.
//
// Terminal state rules:
//   - Complete: traversal finished with no omitted descendants.
//   - Partial: permission/read omissions among descendants (traversal finished).
//   - Incomplete: hard-limit or cooperative cancellation stopped traversal.
//
// Cancellation and hard-limit take precedence over Partial when both apply.
func measureDirectoryTree(ctx context.Context, path string, limit int, onProgress func(measureProgress)) measureOutcome {
	s := &treeMeasurer{
		root:       path,
		limit:      limit,
		skipCounts: map[string]int64{},
		onProgress: onProgress,
	}
	totals := s.measure(ctx, path)
	state := BrowseStateComplete
	switch {
	case s.incomplete:
		state = BrowseStateIncomplete
	case s.hadOmissions:
		state = BrowseStatePartial
	}
	return measureOutcome{
		Totals:     totals,
		SkipCounts: s.skipCounts,
		State:      state,
	}
}

type treeMeasurer struct {
	root         string
	limit        int
	incomplete   bool
	hadOmissions bool
	descendants  int
	skipCounts   map[string]int64
	onProgress   func(measureProgress)
	// progressEvery controls how often onProgress fires by descendant count
	// (independent of wall-clock throttle in the observer).
	// Zero means every successful file/dir contribution.
	progressEvery int
	progressN     int
}

func (s *treeMeasurer) noteSkip(reason string) {
	if reason == "" {
		reason = SkipReasonReadError
	}
	s.hadOmissions = true
	s.skipCounts[reason]++
}

func (s *treeMeasurer) emitProgress(totals Totals) {
	if s.onProgress == nil {
		return
	}
	s.progressN++
	every := s.progressEvery
	if every <= 0 {
		every = 32 // bound callback volume without making cadence correctness
	}
	if s.progressN%every != 0 && !s.incomplete {
		return
	}
	// Copy skip map into a snapshot map for the callback consumer.
	s.onProgress(measureProgress{
		Totals:     totals,
		SkipCounts: copySkipCounts(s.skipCounts),
	})
}

func (s *treeMeasurer) measure(ctx context.Context, path string) Totals {
	select {
	case <-ctx.Done():
		s.incomplete = true
		s.noteSkip(SkipReasonCanceled)
		return Totals{}
	default:
	}

	info, err := browseLstat(path)
	if err != nil {
		s.noteSkip(classifyError(err))
		return Totals{}
	}
	if isReparsePoint(info) || hasReparseAttr(path) {
		// Nested reparse: intentional non-traversal (same as CLI analyze). Do not
		// count as Partial permission/read omission; direct-child reparse is Skipped
		// before recursive measurement starts.
		return Totals{}
	}
	if !info.IsDir() {
		// Leaf progress is reported by the parent after totals.add so Scanning
		// observations always carry cumulative observed bytes for the child root.
		return Totals{Bytes: info.Size(), FileCount: 1}
	}

	totals := Totals{DirectoryCount: 1}
	entries, err := browseReadDir(path)
	if err != nil {
		// Unreadable directory shell: Partial omission of descendants.
		s.noteSkip(classifyError(err))
		return totals
	}

	for _, entry := range entries {
		if s.incomplete {
			break
		}
		select {
		case <-ctx.Done():
			s.incomplete = true
			s.noteSkip(SkipReasonCanceled)
		default:
		}
		if s.incomplete {
			break
		}

		// Count every inspected descendant under this measured directory toward
		// the independent per-child ceiling (default 100_000).
		s.descendants++
		if s.descendants > s.limit {
			s.incomplete = true
			s.noteSkip(SkipReasonHardLimit)
			break
		}

		childPath := filepath.Join(path, entry.Name())
		childTotals := s.measure(ctx, childPath)
		totals.add(childTotals)
		s.emitProgress(totals)
	}
	return totals
}

func aggregatesFromMap(m map[string]int64) []SkipAggregate {
	if len(m) == 0 {
		return nil
	}
	out := make([]SkipAggregate, 0, len(m))
	for reason, count := range m {
		if count <= 0 || reason == "" {
			continue
		}
		out = append(out, SkipAggregate{Reason: reason, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Reason < out[j].Reason
	})
	return out
}

func copySkipCounts(m map[string]int64) map[string]int64 {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// presentationAttributes holds Windows presentation-only flags.
type presentationAttributes struct {
	Hidden  bool
	System  bool
	Reparse bool
}

// filePresentationAttributes is implemented per-platform.
// Windows reads FILE_ATTRIBUTE_*; other platforms return zeros.
func filePresentationAttributes(path string, info os.FileInfo) presentationAttributes {
	return platformPresentationAttributes(path, info)
}

func hasReparseAttr(path string) bool {
	attrs := platformPresentationAttributes(path, nil)
	return attrs.Reparse
}
