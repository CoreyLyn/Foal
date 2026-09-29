package clean

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMeasureExclusiveBytesCountsOnlyFilesExclusiveToCandidate(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "candidate")
	if err := os.Mkdir(candidate, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(candidate, "one")
	if err := os.WriteFile(file, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(file, filepath.Join(candidate, "two")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	bytes, err := measureExclusiveBytes(context.Background(), candidate)
	if err != nil || bytes != 5 {
		t.Fatalf("internal links: bytes=%d err=%v, want 5", bytes, err)
	}
	logical, err := measureBytes(context.Background(), candidate)
	if err != nil || logical != 10 {
		t.Fatalf("Recycle Bin logical bytes=%d err=%v, want 10", logical, err)
	}
	if err := os.Link(file, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	bytes, err = measureExclusiveBytes(context.Background(), candidate)
	if err != nil || bytes != 0 {
		t.Fatalf("external link: bytes=%d err=%v, want 0", bytes, err)
	}
	logical, err = measureBytes(context.Background(), candidate)
	if err != nil || logical != 10 {
		t.Fatalf("Recycle Bin logical bytes with external link=%d err=%v, want 10", logical, err)
	}
}

func TestMeasureExclusiveBytesCanceledBeforeWalkReturnsPromptly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := measureExclusiveBytes(ctx, t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("measureBytes canceled err = %v, want context.Canceled", err)
	}
}
