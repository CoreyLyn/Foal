package status

import (
	"os"
	"runtime"
	"time"
)

const version = "dev"

type Snapshot struct {
	Status    string        `json:"status"`
	Disk      DiskSnapshot  `json:"disk"`
	OS        OSSnapshot    `json:"os"`
	Foal      FoalSnapshot  `json:"foal"`
	ElapsedMS int64         `json:"elapsed_ms"`
	Skipped   []StatusIssue `json:"skipped"`
	Errors    []StatusIssue `json:"errors"`
}

type DiskSnapshot struct {
	Path           string `json:"path"`
	TotalBytes     uint64 `json:"total_bytes,omitempty"`
	FreeBytes      uint64 `json:"free_bytes,omitempty"`
	AvailableBytes uint64 `json:"available_bytes,omitempty"`
}

type OSSnapshot struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
}

type FoalSnapshot struct {
	Name       string `json:"name"`
	Command    string `json:"command"`
	Executable string `json:"executable"`
	Version    string `json:"version"`
}

type StatusIssue struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
}

func Capture() Snapshot {
	return CaptureForDiskPath(os.TempDir())
}

func CaptureForDiskPath(path string) Snapshot {
	start := time.Now()
	skipped := []StatusIssue{}

	disk, err := captureDisk(path)
	if err != nil {
		skipped = append(skipped, StatusIssue{
			Code:        "disk_snapshot_unavailable",
			Message:     err.Error(),
			Recoverable: true,
		})
	}

	return Snapshot{
		Status: "ok",
		Disk:   disk,
		OS: OSSnapshot{
			GOOS:   runtime.GOOS,
			GOARCH: runtime.GOARCH,
		},
		Foal: FoalSnapshot{
			Name:       "Foal",
			Command:    "foal",
			Executable: "foal.exe",
			Version:    version,
		},
		ElapsedMS: time.Since(start).Milliseconds(),
		Skipped:   skipped,
		Errors:    []StatusIssue{},
	}
}
