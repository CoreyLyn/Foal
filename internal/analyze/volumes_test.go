package analyze_test

import (
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/analyze"
)

// fakeDriveProbe is a deterministic DriveProbe for ListLocalVolumes tests.
type fakeDriveProbe struct {
	mask     uint32
	maskErr  error
	kinds    map[string]analyze.VolumeKind // root -> kind; missing = unsupported
	meta     map[string]analyze.VolumeMetadata
	calls    int
	metaRoot []string
}

func (f *fakeDriveProbe) LogicalDriveMask() (uint32, error) {
	f.calls++
	return f.mask, f.maskErr
}

func (f *fakeDriveProbe) SupportedKind(root string) (analyze.VolumeKind, bool) {
	kind, ok := f.kinds[root]
	return kind, ok
}

func (f *fakeDriveProbe) ProbeMetadata(root string) analyze.VolumeMetadata {
	f.metaRoot = append(f.metaRoot, root)
	if meta, ok := f.meta[root]; ok {
		return meta
	}
	return analyze.VolumeMetadata{Available: false}
}

func TestListLocalVolumesIncludesFixedAndRemovableWithMetadata(t *testing.T) {
	probe := &fakeDriveProbe{
		// C: and E: present
		mask: (1 << 2) | (1 << 4),
		kinds: map[string]analyze.VolumeKind{
			`C:\`: analyze.VolumeKindFixed,
			`E:\`: analyze.VolumeKindRemovable,
		},
		meta: map[string]analyze.VolumeMetadata{
			`C:\`: {
				Available:   true,
				Label:       "System",
				FileSystem:  "NTFS",
				TotalBytes:  1000,
				FreeBytes:   400,
				HasCapacity: true,
			},
			`E:\`: {
				Available:   true,
				Label:       "USB",
				FileSystem:  "FAT32",
				TotalBytes:  500,
				FreeBytes:   100,
				HasCapacity: true,
			},
		},
	}

	got := analyze.ListLocalVolumes(probe)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	if got[0].Letter != "C:" || got[0].Kind != analyze.VolumeKindFixed || !got[0].Available {
		t.Fatalf("first volume = %#v", got[0])
	}
	if got[0].Label != "System" || got[0].FileSystem != "NTFS" || got[0].TotalBytes != 1000 || got[0].FreeBytes != 400 {
		t.Fatalf("C: metadata = %#v", got[0])
	}
	if got[1].Letter != "E:" || got[1].Kind != analyze.VolumeKindRemovable {
		t.Fatalf("second volume = %#v", got[1])
	}
}

func TestListLocalVolumesExcludesUnsupportedDriveTypes(t *testing.T) {
	probe := &fakeDriveProbe{
		// C: fixed, D: present but unsupported (optical/network), Z: unsupported
		mask: (1 << 2) | (1 << 3) | (1 << 25),
		kinds: map[string]analyze.VolumeKind{
			`C:\`: analyze.VolumeKindFixed,
			// D: and Z: omitted from kinds → excluded
		},
		meta: map[string]analyze.VolumeMetadata{
			`C:\`: {Available: true, HasCapacity: true, TotalBytes: 1, FreeBytes: 1},
		},
	}

	got := analyze.ListLocalVolumes(probe)
	if len(got) != 1 || got[0].Letter != "C:" {
		t.Fatalf("got %#v, want only C:", got)
	}
	// ProbeMetadata must not be called for unsupported letters.
	for _, root := range probe.metaRoot {
		if root != `C:\` {
			t.Fatalf("ProbeMetadata called for unsupported root %q", root)
		}
	}
}

func TestListLocalVolumesKeepsUnavailableLocalDrives(t *testing.T) {
	probe := &fakeDriveProbe{
		mask: (1 << 2) | (1 << 5), // C: and F:
		kinds: map[string]analyze.VolumeKind{
			`C:\`: analyze.VolumeKindFixed,
			`F:\`: analyze.VolumeKindRemovable,
		},
		meta: map[string]analyze.VolumeMetadata{
			`C:\`: {Available: true, Label: "OS", FileSystem: "NTFS", HasCapacity: true, TotalBytes: 10, FreeBytes: 2},
			`F:\`: {Available: false, Label: "Empty", FileSystem: ""}, // media not ready
		},
	}

	got := analyze.ListLocalVolumes(probe)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 unavailable kept: %#v", len(got), got)
	}
	if got[1].Letter != "F:" || got[1].Available {
		t.Fatalf("unavailable F: = %#v", got[1])
	}
}

func TestListLocalVolumesDoesNotWalkDirectoryContents(t *testing.T) {
	// Contract: only probe methods are used; no recursive scan side effects.
	probe := &fakeDriveProbe{
		mask: 1 << 2,
		kinds: map[string]analyze.VolumeKind{
			`C:\`: analyze.VolumeKindFixed,
		},
		meta: map[string]analyze.VolumeMetadata{
			`C:\`: {Available: true, HasCapacity: true, TotalBytes: 1, FreeBytes: 1},
		},
	}
	_ = analyze.ListLocalVolumes(probe)
	if probe.calls != 1 {
		t.Fatalf("LogicalDriveMask calls = %d, want 1", probe.calls)
	}
	if len(probe.metaRoot) != 1 || probe.metaRoot[0] != `C:\` {
		t.Fatalf("metadata roots = %v, want only C:\\", probe.metaRoot)
	}
}

func TestFocusLocalVolumeIndexPrefersCThenFirstAvailable(t *testing.T) {
	volumes := []analyze.LocalVolume{
		{Letter: "D:", Available: true},
		{Letter: "C:", Available: false},
		{Letter: "E:", Available: true},
	}
	if got := analyze.FocusLocalVolumeIndex(volumes); got != 1 {
		t.Fatalf("focus with C present = %d, want 1", got)
	}

	noC := []analyze.LocalVolume{
		{Letter: "D:", Available: false},
		{Letter: "E:", Available: true},
		{Letter: "F:", Available: true},
	}
	if got := analyze.FocusLocalVolumeIndex(noC); got != 1 {
		t.Fatalf("focus without C = %d, want first available (1)", got)
	}

	none := []analyze.LocalVolume{
		{Letter: "D:", Available: false},
		{Letter: "E:", Available: false},
	}
	if got := analyze.FocusLocalVolumeIndex(none); got != 0 {
		t.Fatalf("focus when none available = %d, want 0", got)
	}

	if got := analyze.FocusLocalVolumeIndex(nil); got != 0 {
		t.Fatalf("focus empty = %d, want 0", got)
	}
}

func TestListLocalVolumesEmptyMask(t *testing.T) {
	got := analyze.ListLocalVolumes(&fakeDriveProbe{mask: 0})
	if len(got) != 0 {
		t.Fatalf("empty mask volumes = %#v", got)
	}
}

func TestListLocalVolumesTrimsLabelAndFileSystem(t *testing.T) {
	probe := &fakeDriveProbe{
		mask: 1 << 2,
		kinds: map[string]analyze.VolumeKind{
			`C:\`: analyze.VolumeKindFixed,
		},
		meta: map[string]analyze.VolumeMetadata{
			`C:\`: {
				Available:   true,
				Label:       "  Data  ",
				FileSystem:  " NTFS ",
				HasCapacity: true,
				TotalBytes:  1,
				FreeBytes:   1,
			},
		},
	}
	got := analyze.ListLocalVolumes(probe)
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Label != "Data" || got[0].FileSystem != "NTFS" {
		t.Fatalf("trimmed fields = label %q fs %q", got[0].Label, got[0].FileSystem)
	}
	if strings.Contains(got[0].Label, " ") {
		t.Fatalf("label still has spaces: %q", got[0].Label)
	}
}
