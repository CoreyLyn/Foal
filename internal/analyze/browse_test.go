package analyze

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestBrowseLocationEnumeratesEveryDirectChild(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "subdir", "nested.txt"), []byte("nested-content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "z.bin"), make([]byte, 100), 0644); err != nil {
		t.Fatal(err)
	}

	result := BrowseLocation(context.Background(), root, BrowseOptions{})
	if !result.OK {
		t.Fatalf("BrowseLocation failed: %#v", result.Reason)
	}
	if result.Root == "" {
		t.Fatal("empty root")
	}
	if len(result.Children) != 3 {
		t.Fatalf("children = %d, want 3: %#v", len(result.Children), result.Children)
	}

	byName := map[string]BrowseChild{}
	for _, c := range result.Children {
		byName[c.Name] = c
	}

	file := byName["a.txt"]
	if file.Kind != BrowseKindFile || file.Bytes != 5 || file.Navigable {
		t.Fatalf("a.txt = %#v", file)
	}
	if file.State != BrowseStateComplete {
		t.Fatalf("a.txt state = %q", file.State)
	}

	big := byName["z.bin"]
	if big.Kind != BrowseKindFile || big.Bytes != 100 || big.Navigable {
		t.Fatalf("z.bin = %#v", big)
	}

	dir := byName["subdir"]
	if dir.Kind != BrowseKindDirectory || !dir.Navigable {
		t.Fatalf("subdir = %#v", dir)
	}
	if dir.Bytes != int64(len("nested-content")) {
		t.Fatalf("subdir bytes = %d, want %d", dir.Bytes, len("nested-content"))
	}
	if dir.FileCount != 1 || dir.DirectoryCount < 1 {
		t.Fatalf("subdir counts = files=%d dirs=%d", dir.FileCount, dir.DirectoryCount)
	}
	if dir.State != BrowseStateComplete {
		t.Fatalf("subdir state = %q", dir.State)
	}
}

func TestBrowseLocationFilesCannotBeEntered(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "only-file"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	result := BrowseLocation(context.Background(), root, BrowseOptions{})
	if !result.OK || len(result.Children) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if result.Children[0].Navigable {
		t.Fatal("files must not be navigable")
	}
}

func TestBrowseLocationIndependentPerChildDescendantLimit(t *testing.T) {
	root := t.TempDir()
	// Two directory children each with enough descendants to hit a low limit.
	// Independent ceilings: both should still be measured (each gets own budget).
	for _, name := range []string{"alpha", "beta"} {
		dir := filepath.Join(root, name)
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 8; i++ {
			if err := os.WriteFile(filepath.Join(dir, "f"+string(rune('a'+i))+".txt"), []byte("data"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Limit of 5 descendants per child directory → both incomplete, both present.
	result := BrowseLocation(context.Background(), root, BrowseOptions{DescendantLimit: 5})
	if !result.OK {
		t.Fatalf("failed: %#v", result.Reason)
	}
	if len(result.Children) != 2 {
		t.Fatalf("children = %d, want 2", len(result.Children))
	}
	for _, c := range result.Children {
		if c.Kind != BrowseKindDirectory {
			t.Fatalf("kind = %q", c.Kind)
		}
		if c.State != BrowseStateIncomplete {
			t.Fatalf("%s state = %q, want incomplete (independent limit)", c.Name, c.State)
		}
		if c.Bytes == 0 {
			t.Fatalf("%s should have partial observed bytes", c.Name)
		}
	}
}

func TestBrowseLocationClassifiesOnlyDirectArtifactChildren(t *testing.T) {
	root := t.TempDir()
	nodeModules := filepath.Join(root, "node_modules")
	if err := os.Mkdir(nodeModules, 0755); err != nil {
		t.Fatal(err)
	}
	// Nested artifact under a normal directory must not produce classification
	// on the parent; only the direct name match is labeled.
	nested := filepath.Join(root, "project")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(nested, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "node_modules", "pkg.js"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeModules, "pkg.js"), []byte("direct"), 0644); err != nil {
		t.Fatal(err)
	}

	result := BrowseLocation(context.Background(), root, BrowseOptions{})
	if !result.OK {
		t.Fatalf("failed: %#v", result.Reason)
	}
	byName := map[string]BrowseChild{}
	for _, c := range result.Children {
		byName[c.Name] = c
	}
	if byName["node_modules"].Classification != "project_artifact_clue" {
		t.Fatalf("direct node_modules classification = %q", byName["node_modules"].Classification)
	}
	if byName["project"].Classification != "" {
		t.Fatalf("project must not be classified from nested artifact; got %q", byName["project"].Classification)
	}
}

func TestBrowseLocationReparseVisibleNotNavigable(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "secret.bin"), make([]byte, 50), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	result := BrowseLocation(context.Background(), root, BrowseOptions{})
	if !result.OK {
		t.Fatalf("failed: %#v", result.Reason)
	}
	var linkChild *BrowseChild
	for i := range result.Children {
		if result.Children[i].Name == "link" {
			linkChild = &result.Children[i]
			break
		}
	}
	if linkChild == nil {
		t.Fatal("reparse child must remain visible")
	}
	if linkChild.Kind != BrowseKindReparse {
		t.Fatalf("kind = %q, want reparse_point", linkChild.Kind)
	}
	if linkChild.Navigable {
		t.Fatal("reparse must not be navigable")
	}
	if linkChild.State != BrowseStateSkipped || linkChild.SkipReason != "reparse_point" {
		t.Fatalf("reparse state/reason = %q/%q", linkChild.State, linkChild.SkipReason)
	}
	// Must not have traversed into the target for size.
	if linkChild.Bytes != 0 {
		t.Fatalf("reparse must not be measured; bytes=%d", linkChild.Bytes)
	}
}

func TestBrowseLocationIncludesHiddenAndSystemPresentationFlags(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows file attributes only")
	}
	root := t.TempDir()
	hiddenPath := filepath.Join(root, "hidden-file.txt")
	if err := os.WriteFile(hiddenPath, []byte("hide"), 0644); err != nil {
		t.Fatal(err)
	}
	setWindowsAttrs(t, hiddenPath, syscall.FILE_ATTRIBUTE_HIDDEN)

	sysPath := filepath.Join(root, "sys-file.txt")
	if err := os.WriteFile(sysPath, []byte("sys"), 0644); err != nil {
		t.Fatal(err)
	}
	setWindowsAttrs(t, sysPath, syscall.FILE_ATTRIBUTE_SYSTEM)

	bothPath := filepath.Join(root, "both.txt")
	if err := os.WriteFile(bothPath, []byte("both"), 0644); err != nil {
		t.Fatal(err)
	}
	setWindowsAttrs(t, bothPath, syscall.FILE_ATTRIBUTE_HIDDEN|syscall.FILE_ATTRIBUTE_SYSTEM)

	result := BrowseLocation(context.Background(), root, BrowseOptions{})
	if !result.OK {
		t.Fatalf("failed: %#v", result.Reason)
	}
	byName := map[string]BrowseChild{}
	for _, c := range result.Children {
		byName[c.Name] = c
	}
	if !byName["hidden-file.txt"].Hidden || byName["hidden-file.txt"].System {
		t.Fatalf("hidden flags: %#v", byName["hidden-file.txt"])
	}
	if byName["sys-file.txt"].Hidden || !byName["sys-file.txt"].System {
		t.Fatalf("system flags: %#v", byName["sys-file.txt"])
	}
	if !byName["both.txt"].Hidden || !byName["both.txt"].System {
		t.Fatalf("both flags: %#v", byName["both.txt"])
	}
	// Presentation only: still measured and listed (not cleanup language / skip).
	if byName["hidden-file.txt"].State != BrowseStateComplete || byName["hidden-file.txt"].Bytes != 4 {
		t.Fatalf("hidden must remain fully visible: %#v", byName["hidden-file.txt"])
	}
}

func setWindowsAttrs(t *testing.T, path string, attrs uint32) {
	t.Helper()
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.SetFileAttributes(ptr, attrs); err != nil {
		t.Fatalf("SetFileAttributes(%s): %v", path, err)
	}
}

func TestBrowseLocationRejectsUnsupportedRoots(t *testing.T) {
	result := BrowseLocation(context.Background(), `\\server\share\x`, BrowseOptions{})
	if result.OK {
		t.Fatal("UNC must fail closed")
	}
	if result.Reason.Code != "unc_path" {
		t.Fatalf("code = %q, want unc_path", result.Reason.Code)
	}
}

func TestBrowseLocationAcceptsWindowsManagedDirectory(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-managed path")
	}
	// Prove Analyze read-root policy accepts Windows-managed trees for browse.
	// Do not fully measure every child under C:\Windows (too heavy for unit tests).
	// Cancel immediately after entry so validation + ReadDir start is enough, then
	// also assert a small temp dir under a managed-style path is not required.
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel before call: BrowseLocation still validates root, then ReadDir may run
	// before the first child loop checks ctx. Use a short deadline instead of full walk.
	cancel()
	result := BrowseLocation(ctx, `C:\Windows`, BrowseOptions{DescendantLimit: 1})
	if !result.OK {
		// Immediate cancel can still fail if ReadDir fails; policy rejections are hard fails.
		if result.Reason.Code == "unsupported_volume" || result.Reason.Code == "unc_path" ||
			result.Reason.Code == "reparse_point" || result.Reason.Code == "device_path" {
			t.Fatalf("Windows-managed root rejected by policy: %#v", result.Reason)
		}
		t.Logf("C:\\Windows not fully browsable here: %#v", result.Reason)
		return
	}
	if result.Root == "" {
		t.Fatal("empty root")
	}
}

func TestBrowseLocationDoesNotPrefetchSiblings(t *testing.T) {
	// Contract: BrowseLocation only reads the supplied root. Documented via API
	// (single root argument). Structural test: two roots measured independently.
	a := t.TempDir()
	b := t.TempDir()
	if err := os.WriteFile(filepath.Join(a, "only-a"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, "only-b"), []byte("bb"), 0644); err != nil {
		t.Fatal(err)
	}
	ra := BrowseLocation(context.Background(), a, BrowseOptions{})
	if !ra.OK || len(ra.Children) != 1 || ra.Children[0].Name != "only-a" {
		t.Fatalf("a = %#v", ra)
	}
	// No evidence of b's children in a's result.
	for _, c := range ra.Children {
		if c.Name == "only-b" || strings.Contains(c.Path, b) {
			t.Fatalf("prefetch of sibling location detected: %#v", c)
		}
	}
}

func TestBrowseLocationNoMutationSurface(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(path, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = BrowseLocation(context.Background(), root, BrowseOptions{})
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "keep" {
		t.Fatalf("browse must not mutate files: %v %q", err, data)
	}
}

func TestBrowseLocationPartialOnPermissionOmission(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "locked")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// Readable nested file so Partial still carries observed lower-bound bytes.
	if err := os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("seen"), 0644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "denied")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}

	origReadDir := browseReadDir
	t.Cleanup(func() { browseReadDir = origReadDir })
	browseReadDir = func(path string) ([]os.DirEntry, error) {
		if filepath.Clean(path) == filepath.Clean(nested) {
			return nil, os.ErrPermission
		}
		return origReadDir(path)
	}

	result := BrowseLocation(context.Background(), root, BrowseOptions{})
	if !result.OK {
		t.Fatalf("failed: %#v", result.Reason)
	}
	var locked *BrowseChild
	for i := range result.Children {
		if result.Children[i].Name == "locked" {
			locked = &result.Children[i]
			break
		}
	}
	if locked == nil {
		t.Fatal("locked child missing")
	}
	if locked.State != BrowseStatePartial {
		t.Fatalf("state = %q, want partial", locked.State)
	}
	if locked.Bytes < int64(len("seen")) {
		t.Fatalf("partial must retain observed bytes, got %d", locked.Bytes)
	}
	if !hasSkipAggregate(locked.SkipAggregates, SkipReasonPermissionDenied) {
		t.Fatalf("aggregates = %#v, want permission_denied", locked.SkipAggregates)
	}
	// No unbounded path list field.
	detail := FormatFocusedDetail(*locked)
	if strings.Contains(detail, nested) {
		t.Fatalf("detail must not list descendant paths: %s", detail)
	}
}

func TestBrowseLocationIncompleteOnHardLimit(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "huge")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		name := filepath.Join(dir, "f"+string(rune('a'+i))+".txt")
		if err := os.WriteFile(name, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	result := BrowseLocation(context.Background(), root, BrowseOptions{DescendantLimit: 5})
	if !result.OK || len(result.Children) != 1 {
		t.Fatalf("result = %#v", result)
	}
	child := result.Children[0]
	if child.State != BrowseStateIncomplete {
		t.Fatalf("state = %q, want incomplete", child.State)
	}
	if child.Bytes == 0 {
		t.Fatal("incomplete should still report observed bytes")
	}
	if !hasSkipAggregate(child.SkipAggregates, SkipReasonHardLimit) {
		t.Fatalf("aggregates = %#v, want hard_limit", child.SkipAggregates)
	}
}

func TestBrowseLocationDirectChildAccessFailureIsSkipped(t *testing.T) {
	root := t.TempDir()
	// Create a real entry so ReadDir lists it, then fail Lstat for that child.
	if err := os.WriteFile(filepath.Join(root, "gone.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(root, "gone.txt")
	origLstat := browseLstat
	t.Cleanup(func() { browseLstat = origLstat })
	browseLstat = func(path string) (os.FileInfo, error) {
		if filepath.Clean(path) == filepath.Clean(gone) {
			return nil, os.ErrPermission
		}
		return origLstat(path)
	}

	result := BrowseLocation(context.Background(), root, BrowseOptions{})
	if !result.OK || len(result.Children) != 1 {
		t.Fatalf("result = %#v", result)
	}
	child := result.Children[0]
	if child.State != BrowseStateSkipped || child.SkipReason != SkipReasonPermissionDenied {
		t.Fatalf("child = %#v, want skipped/permission_denied", child)
	}
}

func TestStreamBrowseLocationEmitsScanningThenTerminal(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "work")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// Enough files to produce at least one progress callback under every=32
	// plus the initial scanning emission and terminal.
	for i := 0; i < 40; i++ {
		if err := os.WriteFile(filepath.Join(dir, "f"+strconv.Itoa(i)+".txt"), []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "solo.txt"), []byte("s"), 0644); err != nil {
		t.Fatal(err)
	}

	var obs []ChildObservation
	result := StreamBrowseLocation(context.Background(), root, BrowseOptions{
		// Disable wall-clock throttle so dense progress can surface when emitted.
		ObservationMinInterval: -1,
	}, func(o ChildObservation) {
		obs = append(obs, o)
	})
	if !result.OK {
		t.Fatalf("failed: %#v", result.Reason)
	}

	// Directory child must have seen Scanning then terminal Complete.
	var workStates []string
	for _, o := range obs {
		if o.Name == "work" {
			workStates = append(workStates, o.State)
		}
	}
	if len(workStates) < 2 {
		t.Fatalf("work observations = %#v, want scanning then terminal", workStates)
	}
	if workStates[0] != BrowseStateScanning {
		t.Fatalf("first work state = %q, want scanning", workStates[0])
	}
	last := workStates[len(workStates)-1]
	if last != BrowseStateComplete || !IsTerminalBrowseState(last) {
		t.Fatalf("last work state = %q, want complete terminal", last)
	}
	// File emits a single terminal complete observation.
	var fileObs []ChildObservation
	for _, o := range obs {
		if o.Name == "solo.txt" {
			fileObs = append(fileObs, o)
		}
	}
	if len(fileObs) != 1 || fileObs[0].State != BrowseStateComplete || !fileObs[0].Terminal {
		t.Fatalf("file obs = %#v", fileObs)
	}
	// Identity fields stable.
	for _, o := range obs {
		if o.Path == "" || o.Name == "" || o.Kind == "" {
			t.Fatalf("observation missing identity: %#v", o)
		}
	}
}

func TestStreamBrowseLocationTerminalAlwaysEmittedDespiteThrottle(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "d")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}

	var terminalStates []string
	_ = StreamBrowseLocation(context.Background(), root, BrowseOptions{
		ObservationMinInterval: time.Hour, // extreme throttle
	}, func(o ChildObservation) {
		if o.Terminal || IsTerminalBrowseState(o.State) {
			terminalStates = append(terminalStates, o.Name+":"+o.State)
		}
	})
	found := false
	for _, s := range terminalStates {
		if s == "d:"+BrowseStateComplete {
			found = true
		}
	}
	if !found {
		t.Fatalf("terminal complete for d missing; got %#v", terminalStates)
	}
}

func TestBrowseLocationCancelProducesIncompleteNotComplete(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "slow")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		if err := os.WriteFile(filepath.Join(dir, "f"+strconv.Itoa(i)+".txt"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Cancel immediately: child measurement should not report complete.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := BrowseLocation(ctx, root, BrowseOptions{})
	if !result.OK {
		t.Fatalf("failed: %#v", result.Reason)
	}
	// May return zero children if cancel hits before any child, or incomplete child.
	for _, c := range result.Children {
		if c.Kind == BrowseKindDirectory && c.State == BrowseStateComplete {
			t.Fatalf("canceled directory must not be complete: %#v", c)
		}
		if c.Kind == BrowseKindDirectory && c.State != BrowseStateIncomplete && c.State != BrowseStateScanning {
			// Terminal after cancel should be incomplete (measurement ran under canceled ctx).
			if c.State == BrowseStatePartial {
				t.Fatalf("cancel must not surface as partial: %#v", c)
			}
		}
	}
}

func TestBrowseChildObservationFieldsOnCompleteDirectory(t *testing.T) {
	root := t.TempDir()
	nodeModules := filepath.Join(root, "node_modules")
	if err := os.Mkdir(nodeModules, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeModules, "pkg.js"), []byte("pkg"), 0644); err != nil {
		t.Fatal(err)
	}
	result := BrowseLocation(context.Background(), root, BrowseOptions{})
	if !result.OK || len(result.Children) != 1 {
		t.Fatalf("result = %#v", result)
	}
	c := result.Children[0]
	if c.Path != nodeModules || c.Kind != BrowseKindDirectory || c.State != BrowseStateComplete {
		t.Fatalf("child = %#v", c)
	}
	if c.Classification != "project_artifact_clue" {
		t.Fatalf("classification = %q", c.Classification)
	}
	if c.FileCount != 1 || c.Bytes != 3 {
		t.Fatalf("counts/bytes = files=%d bytes=%d", c.FileCount, c.Bytes)
	}
}

func hasSkipAggregate(aggs []SkipAggregate, reason string) bool {
	for _, a := range aggs {
		if a.Reason == reason && a.Count > 0 {
			return true
		}
	}
	return false
}
