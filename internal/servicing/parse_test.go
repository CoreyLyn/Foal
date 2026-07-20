package servicing

import (
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

const analyzeHeader = `Deployment Image Servicing and Management tool
Version: 10.0.19041.1

Image Version: 10.0.19041.1

[==========================100.0%==========================]

Component Store (WinSxS) information:

Windows Explorer Reported Size of Component Store : 6.71 GB

Actual Size of Component Store : 6.53 GB

    Shared with Windows : 5.30 GB
    Backups and Disabled Features : 1.00 GB
    Cache and Temporary Data : 0.22 GB

Date of Last Cleanup : 2024-01-01 00:00:00

`

func TestParseComponentStoreAnalysis(t *testing.T) {
	cases := []struct {
		name            string
		body            string
		wantOK          bool
		wantPackages    int
		wantRecommended bool
	}{
		{
			name:            "normal english yes",
			body:            "Number of Reclaimable Packages : 4\nComponent Store Cleanup Recommended : Yes\n",
			wantOK:          true,
			wantPackages:    4,
			wantRecommended: true,
		},
		{
			name:            "zero packages no",
			body:            "Number of Reclaimable Packages : 0\nComponent Store Cleanup Recommended : No\n",
			wantOK:          true,
			wantPackages:    0,
			wantRecommended: false,
		},
		{
			name:            "crlf line endings",
			body:            "Number of Reclaimable Packages : 7\r\nComponent Store Cleanup Recommended : Yes\r\n",
			wantOK:          true,
			wantPackages:    7,
			wantRecommended: true,
		},
		{
			name:            "whitespace variation around colon",
			body:            "   Number of Reclaimable Packages   :   12   \n\tComponent Store Cleanup Recommended\t:\tNo\t\n",
			wantOK:          true,
			wantPackages:    12,
			wantRecommended: false,
		},
		{
			name:   "missing packages field",
			body:   "Component Store Cleanup Recommended : Yes\n",
			wantOK: false,
		},
		{
			name:   "missing recommendation field",
			body:   "Number of Reclaimable Packages : 4\n",
			wantOK: false,
		},
		{
			name:   "duplicate packages same value",
			body:   "Number of Reclaimable Packages : 4\nNumber of Reclaimable Packages : 4\nComponent Store Cleanup Recommended : Yes\n",
			wantOK: false,
		},
		{
			name:   "contradictory duplicate recommendation",
			body:   "Number of Reclaimable Packages : 4\nComponent Store Cleanup Recommended : Yes\nComponent Store Cleanup Recommended : No\n",
			wantOK: false,
		},
		{
			name:   "malformed number",
			body:   "Number of Reclaimable Packages : four\nComponent Store Cleanup Recommended : Yes\n",
			wantOK: false,
		},
		{
			name:   "negative number rejected",
			body:   "Number of Reclaimable Packages : -1\nComponent Store Cleanup Recommended : Yes\n",
			wantOK: false,
		},
		{
			name:   "recommendation not yes or no",
			body:   "Number of Reclaimable Packages : 4\nComponent Store Cleanup Recommended : Maybe\n",
			wantOK: false,
		},
		{
			name:   "localized output fails closed",
			body:   "Anzahl der zurückforderbaren Pakete : 4\nBereinigung des Komponentenspeichers empfohlen : Ja\n",
			wantOK: false,
		},
		{
			name:   "trailing text on packages line rejected",
			body:   "Number of Reclaimable Packages : 4 packages\nComponent Store Cleanup Recommended : Yes\n",
			wantOK: false,
		},
		{
			name:   "empty output",
			body:   "",
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseComponentStoreAnalysis(analyzeHeader + tc.body + "\nThe operation completed successfully.\n")
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if got.reclaimablePackages != tc.wantPackages || got.cleanupRecommended != tc.wantRecommended {
				t.Fatalf("parsed = %#v, want packages=%d recommended=%v", got, tc.wantPackages, tc.wantRecommended)
			}
		})
	}
}

func TestParseComponentStoreAnalysisRejectsOverlongOutput(t *testing.T) {
	body := "Number of Reclaimable Packages : 4\nComponent Store Cleanup Recommended : Yes\n"
	huge := strings.Repeat("x", maxAnalysisOutputBytes) + "\n" + body
	if _, ok := parseComponentStoreAnalysis(huge); ok {
		t.Fatal("over-long output must fail closed")
	}
}

func TestClassifyComponentStoreAnalysis(t *testing.T) {
	ready := analyzeHeader + "Number of Reclaimable Packages : 4\nComponent Store Cleanup Recommended : Yes\n"
	noWorkZero := analyzeHeader + "Number of Reclaimable Packages : 0\nComponent Store Cleanup Recommended : No\n"
	noWorkNotRecommended := analyzeHeader + "Number of Reclaimable Packages : 3\nComponent Store Cleanup Recommended : No\n"
	invalid := analyzeHeader + "Component Store Cleanup Recommended : Yes\n"

	t.Run("ready", func(t *testing.T) {
		res := classifyComponentStoreAnalysis(0, ready)
		if res.Outcome != clean.ServicingOutcomeReady || res.ReclaimablePackages != 4 || !res.CleanupRecommended {
			t.Fatalf("ready = %#v", res)
		}
		if res.ExitCode == nil || *res.ExitCode != 0 {
			t.Fatalf("ready exit = %#v", res.ExitCode)
		}
		if res.Reason != "" {
			t.Fatalf("ready reason = %q, want empty", res.Reason)
		}
	})
	t.Run("no_work zero packages", func(t *testing.T) {
		res := classifyComponentStoreAnalysis(0, noWorkZero)
		if res.Outcome != clean.ServicingOutcomeNoWork || res.ReclaimablePackages != 0 || res.CleanupRecommended {
			t.Fatalf("no_work zero = %#v", res)
		}
	})
	t.Run("no_work not recommended", func(t *testing.T) {
		res := classifyComponentStoreAnalysis(0, noWorkNotRecommended)
		if res.Outcome != clean.ServicingOutcomeNoWork || res.ReclaimablePackages != 3 || res.CleanupRecommended {
			t.Fatalf("no_work not recommended = %#v", res)
		}
	})
	t.Run("invalid output", func(t *testing.T) {
		res := classifyComponentStoreAnalysis(0, invalid)
		if res.Outcome != clean.ServicingOutcomeFailed || res.Reason != clean.ServicingReasonAnalysisOutputInvalid {
			t.Fatalf("invalid = %#v", res)
		}
		if res.ExitCode == nil || *res.ExitCode != 0 {
			t.Fatalf("invalid exit = %#v", res.ExitCode)
		}
	})
	t.Run("nonzero exit fails", func(t *testing.T) {
		res := classifyComponentStoreAnalysis(1, ready)
		if res.Outcome != clean.ServicingOutcomeFailed || res.Reason != clean.ServicingReasonAnalysisFailed {
			t.Fatalf("nonzero exit = %#v", res)
		}
		if res.ExitCode == nil || *res.ExitCode != 1 {
			t.Fatalf("nonzero exit code = %#v", res.ExitCode)
		}
		// A failed analysis never surfaces parsed package evidence.
		if res.ReclaimablePackages != 0 || res.CleanupRecommended {
			t.Fatalf("nonzero exit leaked evidence: %#v", res)
		}
	})
}
