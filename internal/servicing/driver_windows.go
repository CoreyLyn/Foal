//go:build windows

package servicing

import (
	"context"

	"golang.org/x/sys/windows"

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

// driverCleanupDeps are the helper's collaborators for driver-package removal.
// Tests substitute fakes so they never inventory or modify the real driver
// store.
type driverCleanupDeps struct {
	elevated  func() bool
	inventory func(context.Context) (driverstore.Inventory, error)
	remove    func(string) driverstore.RemoveOutcome
	freeBytes func() (uint64, bool)
}

var productionDriverCleanupDeps = driverCleanupDeps{
	elevated:  func() bool { return windows.GetCurrentProcessToken().IsElevated() },
	inventory: driverstore.InspectDisplayPackageIdentities,
	remove:    driverstore.RemovePackage,
	freeBytes: volumeFreeBytes,
}

// runDriverPackageCleanup runs inside the elevated helper with the production
// driver store.
func runDriverPackageCleanup(requested []driverstore.Identity) clean.DriverPackageCleanupResult {
	return runDriverPackageCleanupWith(productionDriverCleanupDeps, requested)
}

// runDriverPackageCleanupWith removes requested packages one at a time. It
// refuses to run without an elevated token. Immediately before each removal it
// takes a fresh inventory and removes the package only while the superseded
// policy still selects it with exactly the requested identity (so a reused
// published name never redirects removal); otherwise the package is reported
// not_eligible and kept. When a fresh inventory fails, that package and every
// later one are reported as not attempted (candidate). Removal goes through
// SetupUninstallOEMInfW without force. Free space is sampled only around the
// removals.
func runDriverPackageCleanupWith(deps driverCleanupDeps, requested []driverstore.Identity) clean.DriverPackageCleanupResult {
	if err := driverstore.ValidateIdentities(requested); err != nil {
		return clean.DriverPackageCleanupResult{Outcome: clean.ServicingOutcomeFailed, Reason: clean.ServicingReasonHelperFailed}
	}
	if !deps.elevated() {
		return clean.DriverPackageCleanupResult{Outcome: clean.ServicingOutcomeFailed, Reason: clean.ServicingReasonElevationFailed}
	}

	beforeFree, beforeOK := deps.freeBytes()
	result := clean.DriverPackageCleanupResult{}
	removed, failed, unattempted := 0, 0, 0
	inventoryFailed := false
	for _, id := range requested {
		outcome := clean.DriverPackageOutcomeCandidate
		if !inventoryFailed {
			inventory, err := deps.inventory(context.Background())
			if err != nil {
				inventoryFailed = true
			} else {
				outcome = clean.DriverPackageOutcomeNotEligible
				if stillSuperseded(inventory, id) {
					switch deps.remove(id.PublishedName) {
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
			}
		}
		if outcome == clean.DriverPackageOutcomeCandidate {
			unattempted++
		}
		result.Packages = append(result.Packages, clean.ServicingDriverPackage{PublishedName: id.PublishedName, Outcome: outcome})
	}
	afterFree, afterOK := deps.freeBytes()
	if removed > 0 && beforeOK && afterOK && afterFree >= beforeFree {
		delta := int64(afterFree - beforeFree)
		result.ObservedFreeBytes = &delta
	}
	switch {
	case failed > 0:
		result.Outcome = clean.ServicingOutcomeFailed
		result.Reason = clean.ServicingReasonCleanupFailed
	case unattempted > 0:
		result.Outcome = clean.ServicingOutcomeFailed
		result.Reason = clean.ServicingReasonAnalysisFailed
	case removed > 0:
		result.Outcome = clean.ServicingOutcomeCompleted
	default:
		result.Outcome = clean.ServicingOutcomeNoWork
	}
	return result
}

// stillSuperseded reports whether the superseded policy selects a package with
// exactly this identity in the inventory.
func stillSuperseded(inventory driverstore.Inventory, id driverstore.Identity) bool {
	for _, pkg := range driverstore.SelectSuperseded(inventory) {
		if id.Matches(pkg) {
			return true
		}
	}
	return false
}
