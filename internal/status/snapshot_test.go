package status

import "testing"

func TestCaptureWithUnavailableDiskPathReportsStructuredSkip(t *testing.T) {
	snapshot := CaptureForDiskPath("bad\x00path")

	if snapshot.Status != "ok" {
		t.Fatalf("status = %q, want ok", snapshot.Status)
	}
	if len(snapshot.Errors) != 0 {
		t.Fatalf("errors = %#v, want empty", snapshot.Errors)
	}
	if len(snapshot.Skipped) != 1 {
		t.Fatalf("skipped = %#v, want one skipped issue", snapshot.Skipped)
	}
	if snapshot.Skipped[0].Code != "disk_snapshot_unavailable" {
		t.Fatalf("skipped[0].code = %q, want disk_snapshot_unavailable", snapshot.Skipped[0].Code)
	}
	if !snapshot.Skipped[0].Recoverable {
		t.Fatal("skipped[0].recoverable = false, want true")
	}
}
