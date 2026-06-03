package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestHelpUsesFoalNamingOnly(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"--help"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{"Foal", "foal", "foal.exe"} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{"Wole", "wole", "Mole for Windows"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("help output contains forbidden legacy text %q:\n%s", forbidden, output)
		}
	}
}

func TestKnownCommandRoutesAsJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"status", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	var got envelope
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if got.Command != "status" {
		t.Fatalf("command = %q, want status", got.Command)
	}
	result, ok := got.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result has type %T, want object", got.Result)
	}
	if result["status"] != "preview" {
		t.Fatalf("result.status = %v, want preview", result["status"])
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestCommandSpecificArgumentsAreRouted(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--dry-run"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Foal clean") {
		t.Fatalf("stdout = %q, want routed clean output", stdout.String())
	}
}

func TestUnknownCommandJSONErrorShape(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"--json", "missing"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("Run returned %d, want %d", code, exitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	var got envelope
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON error: %v\n%s", err, stderr.String())
	}
	if got.Command != "missing" {
		t.Fatalf("command = %q, want missing", got.Command)
	}
	if got.Error == nil {
		t.Fatal("error is nil")
	}
	if got.Error.Code != "unknown_command" {
		t.Fatalf("error.code = %q, want unknown_command", got.Error.Code)
	}
	if got.Error.Message == "" {
		t.Fatal("error.message is empty")
	}
	if !got.Error.Recoverable {
		t.Fatal("error.recoverable = false, want true")
	}
	if got.Error.Command != "missing" {
		t.Fatalf("error.command = %q, want missing", got.Error.Command)
	}
}

func TestNoWoleCompatibilityCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"--json", "wole"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("Run returned %d, want %d", code, exitUsage)
	}
	var got envelope
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON error: %v\n%s", err, stderr.String())
	}
	if got.Error == nil || got.Error.Code != "unknown_command" {
		t.Fatalf("error = %+v, want unknown_command", got.Error)
	}
}
