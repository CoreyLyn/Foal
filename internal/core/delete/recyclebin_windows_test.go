//go:build windows

package delete_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CoreyLyn/Foal/internal/core/delete"
)

func TestWindowsRecycleBinAdapterIntegrationOptIn(t *testing.T) {
	if os.Getenv("FOAL_RUN_RECYCLEBIN_INTEGRATION") != "1" {
		t.Skip("set FOAL_RUN_RECYCLEBIN_INTEGRATION=1 to run the real Recycle Bin integration test")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "foal-recycle-bin-integration.tmp")
	if err := os.WriteFile(path, []byte("recycle"), 0600); err != nil {
		t.Fatal(err)
	}

	adapter := delete.WindowsRecycleBinAdapter{}
	if err := adapter.MoveToRecycleBin(path); err != nil {
		t.Fatalf("MoveToRecycleBin failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("moved path still exists or stat failed unexpectedly: %v", err)
	}
}
