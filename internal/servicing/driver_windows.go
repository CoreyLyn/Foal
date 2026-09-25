//go:build windows

package servicing

import (
	"context"
	"strings"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/driverstore"
)

// analyzeDriverStore runs the non-elevated, in-process superseded display driver
// inventory (ADR 0036). It never requests elevation or mutates anything.
func analyzeDriverStore(ctx context.Context) clean.DriverStoreAnalysisResult {
	inventory, err := driverstore.InspectDisplayPackages(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return clean.DriverStoreAnalysisResult{Outcome: clean.ServicingOutcomeCanceled, Reason: clean.ServicingReasonContextCanceled}
		}
		return clean.DriverStoreAnalysisResult{Outcome: clean.ServicingOutcomeFailed, Reason: clean.ServicingReasonAnalysisFailed}
	}
	candidates := driverstore.SelectSuperseded(inventory)
	if len(candidates) == 0 {
		return clean.DriverStoreAnalysisResult{Outcome: clean.ServicingOutcomeNoWork}
	}
	packages := make([]clean.ServicingDriverPackage, 0, len(candidates))
	for _, pkg := range candidates {
		packages = append(packages, clean.ServicingDriverPackage{
			PublishedName: pkg.PublishedName,
			OriginalName:  pkg.OriginalName,
			Provider:      pkg.Provider,
			DriverDate:    pkg.DriverDate,
			DriverVersion: pkg.DriverVersion,
			Bytes:         pkg.Bytes,
			Outcome:       clean.DriverPackageOutcomeCandidate,
		})
	}
	return clean.DriverStoreAnalysisResult{Outcome: clean.ServicingOutcomeReady, Packages: packages}
}

// runDriverPackageCleanup runs inside the elevated helper. It re-derives the
// inventory and superseded policy itself and removes only requested packages
// that are still eligible, one at a time, through SetupUninstallOEMInfW without
// force. Requested packages that are no longer eligible are reported and kept.
// Free space is sampled only around the removals.
func runDriverPackageCleanup(requested []string) clean.DriverPackageCleanupResult {
	if err := driverstore.ValidateRequest(requested); err != nil {
		return clean.DriverPackageCleanupResult{Outcome: clean.ServicingOutcomeFailed, Reason: clean.ServicingReasonHelperFailed}
	}
	inventory, err := driverstore.InspectDisplayPackages(context.Background())
	if err != nil {
		return clean.DriverPackageCleanupResult{Outcome: clean.ServicingOutcomeFailed, Reason: clean.ServicingReasonAnalysisFailed}
	}
	eligible := map[string]bool{}
	for _, pkg := range driverstore.SelectSuperseded(inventory) {
		eligible[strings.ToLower(pkg.PublishedName)] = true
	}

	beforeFree, beforeOK := volumeFreeBytes()
	result := clean.DriverPackageCleanupResult{}
	removed, failed := 0, 0
	for _, name := range requested {
		outcome := clean.DriverPackageOutcomeNotEligible
		if eligible[strings.ToLower(name)] {
			switch driverstore.RemovePackage(name) {
			case driverstore.RemoveOutcomeRemoved:
				outcome = clean.DriverPackageOutcomeRemoved
				removed++
			case driverstore.RemoveOutcomeInUse:
				outcome = clean.DriverPackageOutcomeInUse
			default:
				outcome = clean.DriverPackageOutcomeFailed
				failed++
			}
		}
		result.Packages = append(result.Packages, clean.ServicingDriverPackage{PublishedName: name, Outcome: outcome})
	}
	afterFree, afterOK := volumeFreeBytes()
	if removed > 0 && beforeOK && afterOK && afterFree >= beforeFree {
		delta := int64(afterFree - beforeFree)
		result.ObservedFreeBytes = &delta
	}
	switch {
	case failed > 0:
		result.Outcome = clean.ServicingOutcomeFailed
		result.Reason = clean.ServicingReasonCleanupFailed
	case removed > 0:
		result.Outcome = clean.ServicingOutcomeCompleted
	default:
		result.Outcome = clean.ServicingOutcomeNoWork
	}
	return result
}
