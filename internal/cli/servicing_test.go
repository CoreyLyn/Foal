package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// TestCleanAllowServicingFlagWiresAuthorization proves --allow-servicing sets
// clean.Options.AllowServicing on execute.
func TestCleanAllowServicingFlagWiresAuthorization(t *testing.T) {
	disableHistoryRecording(t)
	originalExecute := executeClean
	defer func() { executeClean = originalExecute }()

	var captured clean.Options
	executeClean = func(_ context.Context, opts clean.Options) clean.Result {
		captured = opts
		return clean.Result{Status: "ok", Mode: "execute"}
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"clean", "--execute", "--opt-in", clean.CategoryWinSxSComponentStore, "--allow-servicing", "--json"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("Run returned %d; stderr=%q", code, stderr.String())
	}
	if !captured.AllowServicing {
		t.Fatal("AllowServicing not set from --allow-servicing")
	}
	if len(captured.OptIn) != 1 || captured.OptIn[0] != clean.CategoryWinSxSComponentStore {
		t.Fatalf("OptIn = %#v", captured.OptIn)
	}
}

// TestCleanExecuteWithoutAllowServicingDoesNotAuthorize proves servicing stays
// unauthorized without the explicit flag.
func TestCleanExecuteWithoutAllowServicingDoesNotAuthorize(t *testing.T) {
	disableHistoryRecording(t)
	originalExecute := executeClean
	defer func() { executeClean = originalExecute }()

	var captured clean.Options
	executeClean = func(_ context.Context, opts clean.Options) clean.Result {
		captured = opts
		return clean.Result{Status: "ok", Mode: "execute"}
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"clean", "--execute", "--opt-in", clean.CategoryWinSxSComponentStore, "--json"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("Run returned %d; stderr=%q", code, stderr.String())
	}
	if captured.AllowServicing {
		t.Fatal("AllowServicing must stay false without --allow-servicing")
	}
}

// TestCleanAllowServicingIndependentFromAllowPermanent proves the two
// authorizations never imply one another in either direction.
func TestCleanAllowServicingIndependentFromAllowPermanent(t *testing.T) {
	disableHistoryRecording(t)
	originalExecute := executeClean
	defer func() { executeClean = originalExecute }()

	var captured clean.Options
	executeClean = func(_ context.Context, opts clean.Options) clean.Result {
		captured = opts
		return clean.Result{Status: "ok", Mode: "execute"}
	}

	t.Run("permanent alone does not authorize servicing", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"clean", "--execute", "--opt-in", "d3d_shader_cache", "--allow-permanent", "--json"}, &stdout, &stderr)
		if code != exitOK {
			t.Fatalf("Run returned %d; stderr=%q", code, stderr.String())
		}
		if captured.AllowServicing {
			t.Fatal("--allow-permanent must not authorize servicing")
		}
		if !captured.AllowPermanentDeletion {
			t.Fatal("--allow-permanent must authorize permanent deletion")
		}
	})

	t.Run("servicing alone does not authorize permanent", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"clean", "--execute", "--opt-in", clean.CategoryWinSxSComponentStore, "--allow-servicing", "--json"}, &stdout, &stderr)
		if code != exitOK {
			t.Fatalf("Run returned %d; stderr=%q", code, stderr.String())
		}
		if captured.AllowPermanentDeletion {
			t.Fatal("--allow-servicing must not authorize permanent deletion")
		}
		if !captured.AllowServicing {
			t.Fatal("--allow-servicing must authorize servicing")
		}
	})
}

// TestCleanAllowServicingRequiresExecute proves --allow-servicing without a mode
// is a usage error, while it may accompany --dry-run without authorizing.
func TestCleanAllowServicingRequiresExecute(t *testing.T) {
	disableHistoryRecording(t)

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"clean", "--allow-servicing"}, &stdout, &stderr); code == exitOK {
		t.Fatal("clean --allow-servicing without a mode should fail")
	}

	originalDryRun := dryRunClean
	defer func() { dryRunClean = originalDryRun }()
	var captured clean.Options
	dryRunClean = func(_ context.Context, opts clean.Options) clean.Result {
		captured = opts
		return clean.Result{Status: "preview", Mode: "dry_run"}
	}
	stdout.Reset()
	stderr.Reset()
	code := Run([]string{"clean", "--dry-run", "--opt-in", clean.CategoryWinSxSComponentStore, "--allow-servicing", "--json"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("dry-run with --allow-servicing failed: %d stderr=%q", code, stderr.String())
	}
	if captured.AllowServicing != true {
		t.Fatal("dry-run should still carry AllowServicing (it authorizes no mutation there)")
	}
}

// TestCleanExecuteServicingHumanAndJSONOutput proves an execute run surfaces the
// servicing operation in both the human summary and JSON.
func TestCleanExecuteServicingHumanAndJSONOutput(t *testing.T) {
	disableHistoryRecording(t)
	originalExecute := executeClean
	defer func() { executeClean = originalExecute }()

	exit0 := 0
	servicingResult := func() clean.Result {
		return clean.Result{
			Status: "ok",
			Mode:   "execute",
			ServicingOperations: []clean.ServicingOperation{{
				Category:            clean.CategoryWinSxSComponentStore,
				PlannedAction:       clean.PlannedActionInvokeWindowsServicing,
				Capability:          clean.ServicingCapabilityExecuteComponentStoreCleanup,
				ReclaimablePackages: 4,
				CleanupRecommended:  true,
				Outcome:             clean.ServicingOutcomeCompleted,
				ExitCode:            &exit0,
			}},
			Totals: clean.Totals{ServicingOperationCount: 1},
		}
	}
	executeClean = func(_ context.Context, _ clean.Options) clean.Result { return servicingResult() }

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"clean", "--execute", "--opt-in", clean.CategoryWinSxSComponentStore, "--allow-servicing"}, &stdout, &stderr)
		if code != exitOK {
			t.Fatalf("Run returned %d; stderr=%q", code, stderr.String())
		}
		out := stdout.String()
		if !strings.Contains(out, "Windows component store") || !strings.Contains(out, "completed") {
			t.Fatalf("human summary missing servicing line: %q", out)
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"clean", "--execute", "--opt-in", clean.CategoryWinSxSComponentStore, "--allow-servicing", "--json"}, &stdout, &stderr)
		if code != exitOK {
			t.Fatalf("Run returned %d; stderr=%q", code, stderr.String())
		}
		out := stdout.String()
		if !strings.Contains(out, "servicing_operations") || !strings.Contains(out, "execute_component_store_cleanup") {
			t.Fatalf("json missing servicing operation: %q", out)
		}
	})
}

// TestHelpDocumentsAllowServicing proves help lists the new flag and example.
func TestHelpDocumentsAllowServicing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--help"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("help returned %d", code)
	}
	out := stdout.String()
	for _, want := range []string{
		"--allow-servicing",
		"--opt-in winsxs_component_store --allow-servicing",
		"--opt-in winsxs_component_store --opt-in nvidia_installer_cache",
		"--execute --opt-in nvidia_installer_cache",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q", want)
		}
	}
}
