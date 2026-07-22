package analyze

import (
	"strings"
)

// VolumeKind classifies a local drive-letter volume for Analyze drive entry.
// Only fixed and removable local volumes are supported; remote, optical, RAM,
// and unknown types are excluded by the probe.
type VolumeKind string

const (
	VolumeKindFixed     VolumeKind = "fixed"
	VolumeKindRemovable VolumeKind = "removable"
)

// LocalVolume is inexpensive volume metadata for the Analyze drive-entry TUI.
// Enumeration must never scan directory contents.
type LocalVolume struct {
	// Root is the volume root with trailing separator, e.g. `C:\`.
	Root string
	// Letter is the display drive letter with colon, e.g. `C:`.
	Letter string
	Kind   VolumeKind
	// Available is false when the volume is known but not currently readable
	// (for example empty removable media). Unavailable volumes stay listed.
	Available bool
	// Label is the volume label when known; empty when unavailable or unnamed.
	Label string
	// FileSystem is the filesystem name when known (e.g. NTFS).
	FileSystem string
	// TotalBytes and FreeBytes are capacity figures when HasCapacity is true.
	TotalBytes  uint64
	FreeBytes   uint64
	HasCapacity bool
}

// DriveProbe supplies Windows volume APIs for local-volume enumeration.
// Tests inject deterministic fakes; production uses the platform probe.
type DriveProbe interface {
	// LogicalDriveMask returns the GetLogicalDrives bitmask (bit 0 = A:).
	LogicalDriveMask() (uint32, error)
	// SupportedKind reports whether root is a supported fixed/removable volume.
	// Unsupported types (remote, optical, unknown) return ok=false.
	SupportedKind(root string) (kind VolumeKind, ok bool)
	// ProbeMetadata reads label, filesystem, and free/total space without
	// walking directory contents. Available is false when the volume cannot
	// be read for capacity (media not ready, I/O error, etc.).
	ProbeMetadata(root string) VolumeMetadata
}

// VolumeMetadata is optional capacity and naming data for one volume root.
type VolumeMetadata struct {
	Available   bool
	Label       string
	FileSystem  string
	TotalBytes  uint64
	FreeBytes   uint64
	HasCapacity bool
}

// defaultDriveProbe is the platform-specific probe used by ListLocalVolumes
// when no probe is supplied. Tests replace it or pass an explicit probe.
var defaultDriveProbe DriveProbe = platformDriveProbe{}

// ListLocalVolumes enumerates local fixed and removable drive-letter volumes
// with inexpensive metadata. It never scans directory contents, never follows
// reparse points, and excludes network, optical, UNC, and device roots.
//
// When probe is nil, the platform default probe is used.
func ListLocalVolumes(probe DriveProbe) []LocalVolume {
	if probe == nil {
		probe = defaultDriveProbe
	}
	mask, err := probe.LogicalDriveMask()
	if err != nil || mask == 0 {
		return nil
	}

	var volumes []LocalVolume
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		letter := string(rune('A'+i)) + ":"
		root := letter + `\`
		kind, ok := probe.SupportedKind(root)
		if !ok {
			continue
		}
		meta := probe.ProbeMetadata(root)
		volumes = append(volumes, LocalVolume{
			Root:        root,
			Letter:      letter,
			Kind:        kind,
			Available:   meta.Available,
			Label:       strings.TrimSpace(meta.Label),
			FileSystem:  strings.TrimSpace(meta.FileSystem),
			TotalBytes:  meta.TotalBytes,
			FreeBytes:   meta.FreeBytes,
			HasCapacity: meta.HasCapacity,
		})
	}
	return volumes
}

// FocusLocalVolumeIndex returns the cursor index for drive entry.
// Prefer C: when present; otherwise the first available volume; otherwise 0.
func FocusLocalVolumeIndex(volumes []LocalVolume) int {
	if len(volumes) == 0 {
		return 0
	}
	for i, vol := range volumes {
		if strings.EqualFold(vol.Letter, "C:") {
			return i
		}
	}
	for i, vol := range volumes {
		if vol.Available {
			return i
		}
	}
	return 0
}
