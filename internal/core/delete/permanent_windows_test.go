//go:build windows

package delete_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/CoreyLyn/Foal/internal/core/delete"
	"golang.org/x/sys/windows"
)

// TestFilesystemPermanentRemoverDetailedRemoveSkipsLockedFile exercises the real
// continue-on-error walker on Windows. A file held open with a handle that does
// not grant FILE_SHARE_DELETE cannot be removed (sharing violation), simulating a
// file in use by another process. DetailedRemove must skip it, delete the rest of
// the tree, and report PermanentRemovalPartial with accurate deleted-byte
// accounting; the tree remains because the skipped child keeps it non-empty.
func TestFilesystemPermanentRemoverDetailedRemoveSkipsLockedFile(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "dxcache")
	if err := os.MkdirAll(tree, 0700); err != nil {
		t.Fatal(err)
	}
	deletable := filepath.Join(tree, "a.bin")
	if err := os.WriteFile(deletable, []byte("aaaaaa"), 0600); err != nil { // 6 bytes
		t.Fatal(err)
	}
	locked := filepath.Join(tree, "locked.bin")
	if err := os.WriteFile(locked, []byte("bbbb"), 0600); err != nil { // 4 bytes
		t.Fatal(err)
	}

	// Open the locked file denying FILE_SHARE_DELETE so os.Remove fails with a
	// sharing violation. The handle is closed in cleanup to release the file.
	ptr, err := windows.UTF16PtrFromString(locked)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(
		ptr,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, // no FILE_SHARE_DELETE
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Skipf("could not open locked handle (sharing semantics unavailable): %v", err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(handle) })

	removal := delete.FilesystemPermanentRemover{}.DetailedRemove(context.Background(), tree)
	if removal.Outcome != delete.PermanentRemovalPartial {
		t.Fatalf("outcome = %v, want partial", removal.Outcome)
	}
	if removal.DeletedBytes != 6 {
		t.Fatalf("deleted bytes = %d, want 6 (only the deletable file)", removal.DeletedBytes)
	}
	if _, err := os.Lstat(deletable); !os.IsNotExist(err) {
		t.Fatalf("deletable file should have been removed: %v", err)
	}
	if _, err := os.Lstat(locked); err != nil {
		t.Fatalf("locked file should remain: %v", err)
	}
	if _, err := os.Lstat(tree); err != nil {
		t.Fatalf("tree should remain because a skipped child keeps it non-empty: %v", err)
	}
	if len(removal.Skipped) != 1 || removal.Skipped[0].Path != locked {
		t.Fatalf("skipped = %#v, want only the locked file", removal.Skipped)
	}
	if removal.Skipped[0].Reason.Code != "file_in_use" {
		t.Fatalf("skip reason code = %q, want file_in_use", removal.Skipped[0].Reason.Code)
	}
}
