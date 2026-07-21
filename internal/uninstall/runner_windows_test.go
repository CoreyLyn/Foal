//go:build windows

package uninstall

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultUninstallerRunner_QuotedCommandExecution(t *testing.T) {
	tempDir := t.TempDir()

	// Create a dummy batch script with spaces in its path to simulate
	// an uninstaller like "C:\Users\...\Uninstall Bloome.exe".
	scriptPath := filepath.Join(tempDir, "Uninstall App.cmd")
	outputFile := filepath.Join(tempDir, "output.txt")
	scriptContent := "@echo off\r\necho %* > \"" + outputFile + "\"\r\n"

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to create test script: %v", err)
	}

	runner := defaultUninstallerRunner{}
	ctx := context.Background()

	// Form a command line with quoted executable path and arguments:
	// "\"C:\temp\Uninstall App.cmd\" /allusers=0"
	command := `"` + scriptPath + `" /allusers=0`

	res, err := runner.Run(ctx, command)
	if err != nil {
		t.Fatalf("runner.Run unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("runner.Run returned exit code %d, stderr: %q", res.ExitCode, res.Stderr)
	}

	outBytes, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	gotArg := strings.TrimSpace(string(outBytes))
	if gotArg != "/allusers=0" {
		t.Fatalf("expected script argument '/allusers=0', got %q", gotArg)
	}
}

func TestDefaultUninstallerRunner_EmptyCommand(t *testing.T) {
	runner := defaultUninstallerRunner{}
	_, err := runner.Run(context.Background(), "")
	if err == nil {
		t.Fatalf("expected error for empty command string, got nil")
	}
}
