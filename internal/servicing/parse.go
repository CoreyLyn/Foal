// Package servicing implements Foal's Windows component-store servicing gateway
// (ADR 0029). It coordinates an isolated, elevated helper of the same Foal
// executable over a nonce-bound, one-request named pipe and runs only fixed,
// built-in DISM component-store capabilities. This slice ships read-only
// analysis (analyze_component_store); component-store mutation is a later slice.
//
// The package never returns or persists raw DISM output, OS error text, package
// identifiers, a WinSxS path, or a reclaimable-byte estimate. External surfaces
// receive only the parsed English analysis fields and Foal-owned stable
// messages through clean.ServicingAnalysisResult.
package servicing

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// maxAnalysisOutputBytes bounds the DISM output the parser will inspect. The
// helper captures output into a bounded buffer; the parser also refuses to scan
// anything larger so pathological output cannot exhaust memory or hide a second
// field past a huge blob.
const maxAnalysisOutputBytes = 1 << 20 // 1 MiB

// English AnalyzeComponentStore fields. Matching is deliberately strict and
// English-only: DISM is always invoked with /English, so localized or
// reformatted output must fail closed rather than be reinterpreted. Each
// pattern anchors the whole (CRLF-trimmed) line and tolerates only surrounding
// and around-colon whitespace.
var (
	reReclaimablePackages = regexp.MustCompile(`^[ \t]*Number of Reclaimable Packages[ \t]*:[ \t]*([0-9]+)[ \t]*$`)
	reCleanupRecommended  = regexp.MustCompile(`^[ \t]*Component Store Cleanup Recommended[ \t]*:[ \t]*(Yes|No)[ \t]*$`)
)

// componentStoreAnalysis holds the two strictly parsed English fields. It never
// carries a path, raw output, package identifier, or byte estimate.
type componentStoreAnalysis struct {
	reclaimablePackages int
	cleanupRecommended  bool
}

// parseComponentStoreAnalysis parses strict English AnalyzeComponentStore output
// for exactly the reclaimable-package count and the cleanup recommendation. It
// fails closed (ok=false) on missing, duplicate, malformed, contradictory,
// localized, or over-long output. It reads no path, size, or package identity.
func parseComponentStoreAnalysis(stdout string) (componentStoreAnalysis, bool) {
	if len(stdout) > maxAnalysisOutputBytes {
		return componentStoreAnalysis{}, false
	}
	var (
		packages        int
		packagesSeen    int
		recommended     bool
		recommendedSeen int
	)
	for _, rawLine := range strings.Split(stdout, "\n") {
		line := strings.TrimSuffix(rawLine, "\r")
		if m := reReclaimablePackages.FindStringSubmatch(line); m != nil {
			n, err := strconv.Atoi(m[1])
			if err != nil || n < 0 {
				return componentStoreAnalysis{}, false
			}
			packages = n
			packagesSeen++
			continue
		}
		if m := reCleanupRecommended.FindStringSubmatch(line); m != nil {
			recommended = m[1] == "Yes"
			recommendedSeen++
		}
	}
	// Each field must appear exactly once. Missing (0) fails closed; duplicate
	// (>1) fails closed even when values agree, so contradictory or repeated
	// output can never be reinterpreted as approval.
	if packagesSeen != 1 || recommendedSeen != 1 {
		return componentStoreAnalysis{}, false
	}
	return componentStoreAnalysis{reclaimablePackages: packages, cleanupRecommended: recommended}, true
}

// classifyComponentStoreAnalysis applies the fail-closed analysis guard to a
// completed AnalyzeComponentStore run and returns a path-free, byte-free result.
//
//   - A non-zero DISM exit is windows_servicing_analysis_failed.
//   - Missing, duplicate, malformed, contradictory, or localized fields are
//     windows_servicing_analysis_output_invalid.
//   - Exit 0 with positive reclaimable packages and a Yes recommendation is
//     ready.
//   - Exit 0 with zero packages or a No recommendation is no_work.
//
// The DISM exit code is always attached because DISM actually ran to exit.
func classifyComponentStoreAnalysis(exitCode int, stdout string) clean.ServicingAnalysisResult {
	code := exitCode
	if exitCode != 0 {
		return clean.ServicingAnalysisResult{
			Outcome:  clean.ServicingOutcomeFailed,
			Reason:   clean.ServicingReasonAnalysisFailed,
			ExitCode: &code,
		}
	}
	analysis, ok := parseComponentStoreAnalysis(stdout)
	if !ok {
		return clean.ServicingAnalysisResult{
			Outcome:  clean.ServicingOutcomeFailed,
			Reason:   clean.ServicingReasonAnalysisOutputInvalid,
			ExitCode: &code,
		}
	}
	if analysis.reclaimablePackages > 0 && analysis.cleanupRecommended {
		return clean.ServicingAnalysisResult{
			Outcome:             clean.ServicingOutcomeReady,
			ReclaimablePackages: analysis.reclaimablePackages,
			CleanupRecommended:  analysis.cleanupRecommended,
			ExitCode:            &code,
		}
	}
	return clean.ServicingAnalysisResult{
		Outcome:             clean.ServicingOutcomeNoWork,
		ReclaimablePackages: analysis.reclaimablePackages,
		CleanupRecommended:  analysis.cleanupRecommended,
		ExitCode:            &code,
	}
}
