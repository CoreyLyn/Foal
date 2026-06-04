package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/history"
	"github.com/CoreyLyn/Foal/internal/uninstall"
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
	disableHistoryRecording(t)
	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--dry-run", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	var got envelope
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if got.Command != "clean" {
		t.Fatalf("command = %q, want clean", got.Command)
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

func TestStatusJSONReportsReadOnlySystemSnapshot(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"status", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
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
	for _, key := range []string{"disk", "os", "foal", "elapsed_ms", "skipped", "errors"} {
		if _, ok := result[key]; !ok {
			t.Fatalf("result missing %q: %#v", key, result)
		}
	}
	if result["status"] != "ok" {
		t.Fatalf("result.status = %v, want ok", result["status"])
	}
	foal, ok := result["foal"].(map[string]interface{})
	if !ok {
		t.Fatalf("result.foal has type %T, want object", result["foal"])
	}
	if foal["name"] != "Foal" || foal["command"] != "foal" || foal["executable"] != "foal.exe" {
		t.Fatalf("foal state = %#v, want Foal/foal/foal.exe naming", foal)
	}
	encoded := stdout.String()
	for _, forbidden := range []string{"Wole", "wole"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("status JSON contains forbidden legacy text %q:\n%s", forbidden, encoded)
		}
	}
	osInfo, ok := result["os"].(map[string]interface{})
	if !ok {
		t.Fatalf("result.os has type %T, want object", result["os"])
	}
	if osInfo["goos"] == "" || osInfo["goarch"] == "" {
		t.Fatalf("os state = %#v, want goos and goarch", osInfo)
	}
	disk, ok := result["disk"].(map[string]interface{})
	if !ok {
		t.Fatalf("result.disk has type %T, want object", result["disk"])
	}
	if disk["path"] == "" {
		t.Fatalf("disk state = %#v, want path", disk)
	}
	if _, ok := result["elapsed_ms"].(float64); !ok {
		t.Fatalf("elapsed_ms has type %T, want number", result["elapsed_ms"])
	}
	if _, ok := result["skipped"].([]interface{}); !ok {
		t.Fatalf("skipped has type %T, want array", result["skipped"])
	}
	if _, ok := result["errors"].([]interface{}); !ok {
		t.Fatalf("errors has type %T, want array", result["errors"])
	}
}

func TestAnalyzeJSONReportsDirectoryInsight(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "beta.txt"), []byte("beta-data"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer

	code := Run([]string{"analyze", "--json", root}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	if result["status"] != "ok" {
		t.Fatalf("result.status = %v, want ok", result["status"])
	}
	for _, key := range []string{"root", "totals", "top_children", "skipped", "elapsed_ms"} {
		if _, ok := result[key]; !ok {
			t.Fatalf("result missing %q: %#v", key, result)
		}
	}
	totals := result["totals"].(map[string]interface{})
	if totals["file_count"] != float64(2) {
		t.Fatalf("totals.file_count = %v, want 2", totals["file_count"])
	}
	if totals["directory_count"] != float64(2) {
		t.Fatalf("totals.directory_count = %v, want 2", totals["directory_count"])
	}
	if totals["bytes"] != float64(14) {
		t.Fatalf("totals.bytes = %v, want 14", totals["bytes"])
	}
	topChildren := result["top_children"].([]interface{})
	if len(topChildren) == 0 {
		t.Fatal("top_children is empty")
	}
}

func TestAnalyzeJSONReportsReparsePointsAsSkipped(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "inside.txt"), []byte("inside"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	var stdout, stderr bytes.Buffer

	code := Run([]string{"analyze", "--json", root}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	skipped := result["skipped"].([]interface{})
	if len(skipped) == 0 {
		t.Fatalf("skipped is empty; result=%#v", result)
	}
	first := skipped[0].(map[string]interface{})
	if first["reason"] != "reparse_point" {
		t.Fatalf("skipped[0].reason = %v, want reparse_point", first["reason"])
	}
}

func TestAnalyzeJSONReportsMissingRootAsSkippedWithStableExit(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	var stdout, stderr bytes.Buffer

	code := Run([]string{"analyze", "--json", missing}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	skipped := result["skipped"].([]interface{})
	if len(skipped) != 1 {
		t.Fatalf("len(skipped) = %d, want 1; result=%#v", len(skipped), result)
	}
	first := skipped[0].(map[string]interface{})
	if first["reason"] != "not_found" {
		t.Fatalf("skipped[0].reason = %v, want not_found", first["reason"])
	}
}

func TestUninstallJSONReportsPreviewOnlyReviewContract(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"uninstall", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	if result["status"] != "preview" {
		t.Fatalf("result.status = %v, want preview", result["status"])
	}
	for _, key := range []string{
		"applications",
		"evidence_sources",
		"possible_leftovers",
		"shared_state_concerns",
		"unknown_state",
		"skipped",
		"execution",
	} {
		if _, ok := result[key]; !ok {
			t.Fatalf("result missing %q: %#v", key, result)
		}
	}
	execution, ok := result["execution"].(map[string]interface{})
	if !ok {
		t.Fatalf("execution has type %T, want object", result["execution"])
	}
	if execution["allowed"] != false {
		t.Fatalf("execution.allowed = %v, want false", execution["allowed"])
	}
	actions, ok := execution["actions"].([]interface{})
	if !ok {
		t.Fatalf("execution.actions has type %T, want array", execution["actions"])
	}
	if len(actions) != 0 {
		t.Fatalf("execution.actions = %#v, want empty", actions)
	}
}

func TestUninstallJSONReportsRegistryDiscoveredApplications(t *testing.T) {
	original := reviewUninstall
	reviewUninstall = func() uninstall.Result {
		return uninstall.Result{
			Status: "preview",
			Applications: []uninstall.Application{{
				Name:       "Registry App",
				Version:    "2.4.6",
				Publisher:  "Registry Publisher",
				Evidence:   []string{"windows_registry_uninstall_keys:HKLM64"},
				Confidence: "medium",
				Ownership:  "unknown",
			}},
			EvidenceSources: []uninstall.EvidenceSource{{
				Source: "windows_registry_uninstall_keys:HKLM64",
				Status: "reported",
			}},
			PossibleLeftovers:   []uninstall.LeftoverCandidate{},
			SharedStateConcerns: []uninstall.SharedStateConcern{},
			UnknownState:        []uninstall.UnknownStateCandidate{},
			Skipped:             []uninstall.SkippedReason{},
			Execution: uninstall.ExecutionPolicy{
				Allowed: false,
				Actions: []string{},
				Reason:  "uninstall is preview-only",
			},
		}
	}
	t.Cleanup(func() { reviewUninstall = original })

	var stdout, stderr bytes.Buffer

	code := Run([]string{"uninstall", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	applications := result["applications"].([]interface{})
	if len(applications) != 1 {
		t.Fatalf("applications = %#v, want one registry app", applications)
	}
	app := applications[0].(map[string]interface{})
	if app["name"] != "Registry App" || app["version"] != "2.4.6" || app["publisher"] != "Registry Publisher" {
		t.Fatalf("app = %#v, want registry app metadata", app)
	}
	evidence := app["evidence"].([]interface{})
	if len(evidence) != 1 || evidence[0] != "windows_registry_uninstall_keys:HKLM64" {
		t.Fatalf("evidence = %#v, want registry evidence source", evidence)
	}
	sources := result["evidence_sources"].([]interface{})
	firstSource := sources[0].(map[string]interface{})
	if firstSource["source"] != "windows_registry_uninstall_keys:HKLM64" || firstSource["status"] != "reported" {
		t.Fatalf("evidence source = %#v, want reported registry source", firstSource)
	}
	execution := result["execution"].(map[string]interface{})
	if execution["allowed"] != false {
		t.Fatalf("execution.allowed = %v, want false", execution["allowed"])
	}
	if actions := execution["actions"].([]interface{}); len(actions) != 0 {
		t.Fatalf("execution.actions = %#v, want empty", actions)
	}
}

func TestUninstallJSONReviewSectionsUsePreviewTerminology(t *testing.T) {
	original := reviewUninstall
	reviewUninstall = func() uninstall.Result {
		return uninstall.Result{
			Status: "preview",
			Applications: []uninstall.Application{{
				Name:       "Registry App",
				Version:    "2.4.6",
				Publisher:  "Registry Publisher",
				Evidence:   []string{"windows_registry_uninstall_keys:HKLM64"},
				Confidence: "medium",
				Ownership:  "unknown",
			}},
			EvidenceSources: []uninstall.EvidenceSource{{
				Source: "windows_registry_uninstall_keys:HKLM64",
				Status: "reported",
			}, {
				Source: "known_leftover_locations",
				Status: "skipped",
				Reason: "discovery provider not implemented",
			}},
			PossibleLeftovers:   []uninstall.LeftoverCandidate{},
			SharedStateConcerns: []uninstall.SharedStateConcern{},
			UnknownState:        []uninstall.UnknownStateCandidate{},
			Skipped: []uninstall.SkippedReason{{
				Source:      "known_leftover_locations",
				Reason:      "discovery_provider_not_implemented",
				Recoverable: true,
			}},
			Execution: uninstall.ExecutionPolicy{
				Allowed: false,
				Actions: []string{},
				Reason:  "uninstall is preview-only",
			},
		}
	}
	t.Cleanup(func() { reviewUninstall = original })

	var stdout, stderr bytes.Buffer

	code := Run([]string{"uninstall", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	sections, ok := result["review_sections"].([]interface{})
	if !ok {
		t.Fatalf("review_sections has type %T, want array", result["review_sections"])
	}
	wantSections := map[string]float64{
		"applications":              1,
		"evidence_sources":          2,
		"possible_leftovers":        0,
		"shared_state_concerns":     0,
		"unknown_state":             0,
		"skipped_discovery_sources": 1,
	}
	if len(sections) != len(wantSections) {
		t.Fatalf("review_sections = %#v, want %d sections", sections, len(wantSections))
	}
	for _, sectionValue := range sections {
		section := sectionValue.(map[string]interface{})
		id, _ := section["id"].(string)
		wantCount, ok := wantSections[id]
		if !ok {
			t.Fatalf("section id = %q, want one of %#v", id, wantSections)
		}
		if section["label"] == "" {
			t.Fatalf("section = %#v, want display label", section)
		}
		if section["count"] != wantCount {
			t.Fatalf("section %q count = %v, want %v", id, section["count"], wantCount)
		}
		termText := strings.ToLower(id + " " + section["label"].(string))
		for _, forbidden := range []string{"execute", "execution", "action", "delete", "deletion", "stop process", "uninstall command"} {
			if strings.Contains(termText, forbidden) {
				t.Fatalf("review section uses execution terminology %q in %#v", forbidden, section)
			}
		}
	}
	execution := result["execution"].(map[string]interface{})
	if execution["allowed"] != false {
		t.Fatalf("execution.allowed = %v, want false", execution["allowed"])
	}
	if actions := execution["actions"].([]interface{}); len(actions) != 0 {
		t.Fatalf("execution.actions = %#v, want empty", actions)
	}
}

func TestCommandSpecificArgumentsAreRouted(t *testing.T) {
	disableHistoryRecording(t)
	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--dry-run"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Foal clean") {
		t.Fatalf("stdout = %q, want routed clean output", stdout.String())
	}
}

func TestCleanDryRunNonJSONRendersPreviewReadModelReport(t *testing.T) {
	disableHistoryRecording(t)
	originalDryRun := dryRunClean
	defer func() { dryRunClean = originalDryRun }()

	dryRunClean = func(ctx context.Context, opts clean.Options) clean.Result {
		return clean.Result{
			Status: "preview",
			Mode:   "dry_run",
			DefaultRuleCatalog: []clean.RuleSummary{{
				ID:             "foal_owned_temp_sandboxes",
				Description:    "Foal-owned temporary sandbox entries",
				DefaultEnabled: true,
			}},
			Candidates: []clean.CandidatePreview{{
				Path:          `C:\Users\corey\AppData\Local\Temp\foal-preview.tmp`,
				Bytes:         12,
				Rule:          "foal_owned_temp_sandboxes",
				PlannedAction: "move_to_recycle_bin",
			}},
			Skipped:          []clean.SkippedItem{},
			DetailedListPath: `C:\Users\corey\AppData\Roaming\Foal\history\clean-dry-run-detail.txt`,
		}
	}

	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--dry-run"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Foal clean",
		"Preview only",
		"Protection rules",
		"Potential space: 12 bytes",
		`C:\Users\corey\AppData\Local\Temp\foal-preview.tmp`,
		`Detailed candidate list: C:\Users\corey\AppData\Roaming\Foal\history\clean-dry-run-detail.txt`,
		"No changes were made",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{"Whitelist", "Mole for Windows", "Wole", "wole"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output contains forbidden text %q:\n%s", forbidden, output)
		}
	}
}

func TestCleanDryRunNonJSONGroupsSkippedErrorsAndPermissionBoundary(t *testing.T) {
	disableHistoryRecording(t)
	originalDryRun := dryRunClean
	defer func() { dryRunClean = originalDryRun }()

	dryRunClean = func(ctx context.Context, opts clean.Options) clean.Result {
		return clean.Result{
			Status: "preview",
			Mode:   "dry_run",
			DefaultRuleCatalog: []clean.RuleSummary{{
				ID:             "foal_owned_temp_sandboxes",
				Description:    "Foal-owned temporary sandbox entries",
				DefaultEnabled: true,
			}},
			Candidates: []clean.CandidatePreview{{
				Path:          `C:\Users\corey\AppData\Local\Temp\foal-preview.tmp`,
				Bytes:         12,
				Rule:          "foal_owned_temp_sandboxes",
				PlannedAction: "move_to_recycle_bin",
			}},
			Skipped: []clean.SkippedItem{{
				Path:  `\\?\C:\Windows\System32`,
				Bytes: 4096,
				Rule:  "foal_owned_temp_sandboxes",
				Reason: clean.StructuredIssue{
					Code:        "protected_path",
					Message:     "protected Windows location",
					Recoverable: true,
					Path:        `\\?\C:\Windows\System32`,
					Rule:        "foal_owned_temp_sandboxes",
				},
			}},
			Errors: []clean.StructuredIssue{{
				Code:        "inspection_failed",
				Message:     "could not inspect root",
				Recoverable: true,
				Path:        `C:\Users\corey\AppData\Local\Temp\foal-missing-root`,
				Rule:        "foal_owned_temp_sandboxes",
			}},
		}
	}

	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--dry-run"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Potential space: 12 bytes",
		"Permission boundary",
		"Skipped items",
		`\\?\C:\Windows\System32`,
		"protected_path",
		"Inspection errors",
		`C:\Users\corey\AppData\Local\Temp\foal-missing-root`,
		"inspection_failed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{"Potential space: 4108 bytes", "Run as Administrator", "run as administrator"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output contains forbidden text %q:\n%s", forbidden, output)
		}
	}
}

func TestCleanDryRunNonJSONIsReadableWithoutSymbolsOrColor(t *testing.T) {
	disableHistoryRecording(t)
	originalDryRun := dryRunClean
	defer func() { dryRunClean = originalDryRun }()

	dryRunClean = func(ctx context.Context, opts clean.Options) clean.Result {
		return clean.Result{
			Status: "preview",
			Mode:   "dry_run",
			DefaultRuleCatalog: []clean.RuleSummary{{
				ID:             "foal_owned_temp_sandboxes",
				Description:    "Foal-owned temporary sandbox entries",
				DefaultEnabled: true,
			}},
			Candidates: []clean.CandidatePreview{{
				Path:          `C:\Users\corey\AppData\Local\Temp\foal-readable.tmp`,
				Bytes:         12,
				Rule:          "foal_owned_temp_sandboxes",
				PlannedAction: "move_to_recycle_bin",
			}},
			Skipped: []clean.SkippedItem{{
				Path:  `\\?\C:\Windows\System32`,
				Bytes: 4096,
				Rule:  "foal_owned_temp_sandboxes",
				Reason: clean.StructuredIssue{
					Code:        "protected_path",
					Message:     "protected Windows location",
					Recoverable: true,
					Path:        `\\?\C:\Windows\System32`,
					Rule:        "foal_owned_temp_sandboxes",
				},
			}},
			Errors: []clean.StructuredIssue{{
				Code:        "inspection_failed",
				Message:     "could not inspect root",
				Recoverable: true,
				Path:        `C:\Users\corey\AppData\Local\Temp\foal-missing-root`,
				Rule:        "foal_owned_temp_sandboxes",
			}},
			DetailedListPath: `C:\Users\corey\AppData\Roaming\Foal\history\clean-dry-run-detail.txt`,
		}
	}

	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--dry-run"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Foal clean",
		"Preview only",
		"foal clean --execute",
		"Detailed candidate list:",
		"status: default candidate",
		"status: skipped",
		"status: inspection error",
		"planned action: Recycle Bin",
		"reason: protected_path",
		"error: inspection_failed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("plain output missing %q:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{
		"✓",
		"✔",
		"✗",
		"✖",
		"⚠",
		"●",
		"•",
		"→",
		"Whitelist",
		"Mole for Windows",
		"Wole",
		"wole",
		"Run as Administrator",
		"run as administrator",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("plain output contains forbidden presentation or product text %q:\n%s", forbidden, output)
		}
	}
}

func TestCleanDryRunJSONDoesNotIncludeHumanReportText(t *testing.T) {
	disableHistoryRecording(t)
	originalDryRun := dryRunClean
	defer func() { dryRunClean = originalDryRun }()

	dryRunClean = func(ctx context.Context, opts clean.Options) clean.Result {
		return clean.Result{
			Status: "preview",
			Mode:   "dry_run",
			DefaultRuleCatalog: []clean.RuleSummary{{
				ID:             "foal_owned_temp_sandboxes",
				Description:    "Foal-owned temporary sandbox entries",
				DefaultEnabled: true,
			}},
			Candidates: []clean.CandidatePreview{{
				Path:          `C:\Users\corey\AppData\Local\Temp\foal-preview.tmp`,
				Bytes:         12,
				Rule:          "foal_owned_temp_sandboxes",
				PlannedAction: "move_to_recycle_bin",
			}},
			Skipped: []clean.SkippedItem{{
				Path:  `\\?\C:\Windows\System32`,
				Bytes: 4096,
				Rule:  "foal_owned_temp_sandboxes",
				Reason: clean.StructuredIssue{
					Code:        "protected_path",
					Message:     "protected Windows location",
					Recoverable: true,
					Path:        `\\?\C:\Windows\System32`,
					Rule:        "foal_owned_temp_sandboxes",
				},
			}},
			Errors: []clean.StructuredIssue{{
				Code:        "inspection_failed",
				Message:     "could not inspect root",
				Recoverable: true,
				Path:        `C:\Users\corey\AppData\Local\Temp\foal-missing-root`,
				Rule:        "foal_owned_temp_sandboxes",
			}},
			Totals: clean.Totals{CandidateCount: 1, CandidateBytes: 12},
		}
	}

	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--dry-run", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	if result["status"] != "preview" || result["mode"] != "dry_run" {
		t.Fatalf("result status/mode = %v/%v, want preview/dry_run", result["status"], result["mode"])
	}
	encoded := stdout.String()
	for _, forbidden := range []string{"Protection rules", "Potential space", "No changes were made", "Preview only", "Permission boundary", "Skipped items", "Inspection errors", "status: default candidate", "status: skipped", "status: inspection error"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("JSON contains human report text %q:\n%s", forbidden, encoded)
		}
	}
}

func TestCleanDryRunJSONDoesNotWriteOrExposeDetailedCandidateList(t *testing.T) {
	historyDir := t.TempDir()
	t.Setenv("FOAL_HISTORY_DIR", historyDir)
	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--dry-run", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	encoded := stdout.String()
	if strings.Contains(encoded, "detailed_list_path") || strings.Contains(encoded, "Detailed candidate list") {
		t.Fatalf("JSON leaked detailed list metadata:\n%s", encoded)
	}
	for _, name := range readDirNames(t, historyDir) {
		if strings.HasSuffix(name, "-detailed-candidates.txt") {
			t.Fatalf("JSON dry-run wrote detailed candidate list %q; files=%v", name, readDirNames(t, historyDir))
		}
	}
}

func TestCleanDryRunNonJSONWritesAndDisplaysDetailedCandidateList(t *testing.T) {
	historyDir := t.TempDir()
	t.Setenv("FOAL_HISTORY_DIR", historyDir)
	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--dry-run"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "Detailed candidate list: ") {
		t.Fatalf("output missing detailed candidate list path:\n%s", output)
	}
	var detailedLists []string
	for _, name := range readDirNames(t, historyDir) {
		if strings.HasSuffix(name, "-detailed-candidates.txt") {
			detailedLists = append(detailedLists, name)
		}
	}
	if len(detailedLists) != 1 {
		t.Fatalf("detailed list files = %v, want one; all files=%v", detailedLists, readDirNames(t, historyDir))
	}
	path := filepath.Join(historyDir, detailedLists[0])
	if !strings.Contains(output, path) {
		t.Fatalf("output = %q, want detailed list path %q", output, path)
	}
}

func TestCleanRequiresDryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--json"}, &stdout, &stderr)

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
	if got.Error == nil || got.Error.Code != "invalid_clean_invocation" {
		t.Fatalf("error = %+v, want invalid_clean_invocation", got.Error)
	}
}

func TestCleanExecuteJSONRoutesConfirmedExecution(t *testing.T) {
	disableHistoryRecording(t)
	originalExecute := executeClean
	defer func() { executeClean = originalExecute }()

	called := false
	executeClean = func(ctx context.Context, opts clean.Options) clean.Result {
		called = true
		return clean.Result{
			Status:     "ok",
			Mode:       "execute",
			Candidates: []clean.CandidatePreview{},
			Deleted: []clean.DeletedItem{{
				Path:  `C:\Users\corey\AppData\Local\Temp\foal-owned.tmp`,
				Bytes: 5,
				Rule:  "foal_owned_temp_sandboxes",
			}},
			Skipped: []clean.SkippedItem{},
			Errors:  []clean.StructuredIssue{},
			Totals: clean.Totals{
				CandidateCount: 1,
				DeletedCount:   1,
				AffectedBytes:  5,
			},
		}
	}

	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--execute", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if !called {
		t.Fatal("executeClean was not called")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	if result["status"] != "ok" || result["mode"] != "execute" {
		t.Fatalf("result status/mode = %v/%v, want ok/execute", result["status"], result["mode"])
	}
	deleted := result["deleted"].([]interface{})
	if len(deleted) != 1 {
		t.Fatalf("deleted = %#v, want one item", deleted)
	}
}

func TestCleanDryRunCreatesHistoryWithCommandParameters(t *testing.T) {
	original := newHistoryRecorder
	historyDir := t.TempDir()
	newHistoryRecorder = func() (history.Recorder, error) {
		recorder := history.NewFileRecorder(historyDir)
		return recorder, nil
	}
	t.Cleanup(func() { newHistoryRecorder = original })

	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--dry-run", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	files, err := os.ReadDir(historyDir)
	if err != nil {
		t.Fatalf("read history dir: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("history files = %v, want one session file", files)
	}
	data, err := os.ReadFile(filepath.Join(historyDir, files[0].Name()))
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatal("history file is empty")
	}
	var record history.Record
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("decode session record: %v\n%s", err, lines[0])
	}
	if record.Type != "session" || record.Session == nil {
		t.Fatalf("record = %#v, want session", record)
	}
	if record.Session.Command.Command != "clean" || record.Session.Mode != "dry_run" {
		t.Fatalf("session = %#v, want clean dry_run", record.Session)
	}
	if got := record.Session.Command.Args; len(got) != 3 || got[0] != "clean" || got[1] != "--dry-run" || got[2] != "--json" {
		t.Fatalf("args = %#v, want original clean invocation args", got)
	}
}

func TestHistoryJSONReportsEmptyHistory(t *testing.T) {
	t.Setenv("FOAL_HISTORY_DIR", filepath.Join(t.TempDir(), "missing-history"))
	var stdout, stderr bytes.Buffer

	code := Run([]string{"history", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	if result["status"] != "ok" {
		t.Fatalf("status = %v, want ok", result["status"])
	}
	if sessions := result["sessions"].([]interface{}); len(sessions) != 0 {
		t.Fatalf("sessions = %#v, want empty", sessions)
	}
	if errors := result["errors"].([]interface{}); len(errors) != 0 {
		t.Fatalf("errors = %#v, want empty", errors)
	}
}

func TestHistoryJSONReportsRecentSessionsAndItemOutcomes(t *testing.T) {
	historyDir := t.TempDir()
	t.Setenv("FOAL_HISTORY_DIR", historyDir)
	recorder := history.NewFileRecorder(historyDir)
	itemBytes := int64(12)
	skippedBytes := int64(7)
	dryRunSession := history.SessionRecord{
		ID:        "clean-dry-run",
		Command:   history.CommandParameters{Command: "clean", Args: []string{"clean", "--dry-run", "--json"}},
		Mode:      "dry_run",
		StartedAt: mustTime(t, "2026-06-03T09:00:00Z"),
		EndedAt:   mustTime(t, "2026-06-03T09:00:01Z"),
		Aggregate: history.AggregateOutcomes{CandidateCount: 1, CandidateBytes: itemBytes},
	}
	executeSession := history.SessionRecord{
		ID:        "clean-execute",
		Command:   history.CommandParameters{Command: "clean", Args: []string{"clean", "--execute", "--json"}},
		Mode:      "execute",
		StartedAt: mustTime(t, "2026-06-03T10:00:00Z"),
		EndedAt:   mustTime(t, "2026-06-03T10:00:01Z"),
		Aggregate: history.AggregateOutcomes{DeletedCount: 1, SkippedCount: 1, ErrorCount: 1, AffectedBytes: itemBytes},
	}
	if err := recorder.Record(context.Background(), dryRunSession, []history.ItemRecord{{
		Path:          `C:\Users\corey\AppData\Local\Temp\foal-candidate.tmp`,
		Rule:          "foal_owned_temp_sandboxes",
		PlannedAction: "move_to_recycle_bin",
		Bytes:         &itemBytes,
		Result:        "candidate",
	}}); err != nil {
		t.Fatalf("record dry-run history: %v", err)
	}
	if err := recorder.Record(context.Background(), executeSession, []history.ItemRecord{
		{
			Path:   `C:\Users\corey\AppData\Local\Temp\foal-deleted.tmp`,
			Rule:   "foal_owned_temp_sandboxes",
			Action: "move_to_recycle_bin",
			Bytes:  &itemBytes,
			Result: "deleted",
		},
		{
			Path:          `C:\Users\corey\AppData\Local\Temp\foal-skipped.tmp`,
			Rule:          "foal_owned_temp_sandboxes",
			PlannedAction: "move_to_recycle_bin",
			Bytes:         &skippedBytes,
			Result:        "skipped",
			SkippedReason: &history.Issue{Code: "permission_denied", Message: "access denied", Recoverable: true},
		},
		{
			Path:   `C:\Users\corey\AppData\Local\Temp\foal-error.tmp`,
			Rule:   "foal_owned_temp_sandboxes",
			Result: "error",
			Error:  &history.Issue{Code: "inspection_failed", Message: "inspection failed", Recoverable: true},
		},
	}); err != nil {
		t.Fatalf("record execute history: %v", err)
	}

	var stdout, stderr bytes.Buffer

	code := Run([]string{"history", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	if result["status"] != "ok" {
		t.Fatalf("status = %v, want ok", result["status"])
	}
	sessions := result["sessions"].([]interface{})
	if len(sessions) != 2 {
		t.Fatalf("sessions = %#v, want two sessions", sessions)
	}
	latest := sessions[0].(map[string]interface{})
	if latest["id"] != "clean-execute" || latest["mode"] != "execute" {
		t.Fatalf("latest session = %#v, want execute session first", latest)
	}
	items := latest["items"].([]interface{})
	if len(items) != 3 {
		t.Fatalf("items = %#v, want deleted/skipped/error items", items)
	}
	deleted := items[0].(map[string]interface{})
	if deleted["path"] == "" || deleted["rule"] != "foal_owned_temp_sandboxes" || deleted["action"] != "move_to_recycle_bin" || deleted["result"] != "deleted" {
		t.Fatalf("deleted item = %#v, want path/rule/action/result metadata", deleted)
	}
	skipped := items[1].(map[string]interface{})
	if skipped["planned_action"] != "move_to_recycle_bin" || skipped["bytes"] != float64(skippedBytes) {
		t.Fatalf("skipped item = %#v, want planned action and bytes", skipped)
	}
	reason := skipped["skipped_reason"].(map[string]interface{})
	if reason["code"] != "permission_denied" {
		t.Fatalf("skipped reason = %#v, want permission_denied", reason)
	}
	errorItem := items[2].(map[string]interface{})
	if errorItem["result"] != "error" || errorItem["error"].(map[string]interface{})["code"] != "inspection_failed" {
		t.Fatalf("error item = %#v, want structured error", errorItem)
	}
	older := sessions[1].(map[string]interface{})
	if older["mode"] != "dry_run" {
		t.Fatalf("older session mode = %v, want dry_run", older["mode"])
	}
}

func TestHistoryJSONReportsMalformedHistoryErrors(t *testing.T) {
	historyDir := t.TempDir()
	t.Setenv("FOAL_HISTORY_DIR", historyDir)
	if err := os.WriteFile(filepath.Join(historyDir, "broken.jsonl"), []byte("{not-json\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	code := Run([]string{"history", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	if result["status"] != "partial" {
		t.Fatalf("status = %v, want partial", result["status"])
	}
	errors := result["errors"].([]interface{})
	if len(errors) != 1 {
		t.Fatalf("errors = %#v, want one error", errors)
	}
	first := errors[0].(map[string]interface{})
	if first["code"] != "history_decode_failed" || first["recoverable"] != true {
		t.Fatalf("error = %#v, want recoverable history_decode_failed", first)
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

func readResultObject(t *testing.T, data []byte) map[string]interface{} {
	t.Helper()

	var got envelope
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, string(data))
	}
	result, ok := got.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result has type %T, want object", got.Result)
	}
	return result
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}

func disableHistoryRecording(t *testing.T) {
	t.Helper()
	original := newHistoryRecorder
	newHistoryRecorder = func() (history.Recorder, error) {
		return nil, nil
	}
	t.Cleanup(func() { newHistoryRecorder = original })
}

func readDirNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read dir %q: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
