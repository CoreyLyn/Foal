package clean

import (
	"context"
	"strings"

	"github.com/CoreyLyn/Foal/internal/driverstore"
)

// analyzeDriverPackageCategory runs the non-elevated superseded display driver
// inventory through the gateway. It never opens UAC. An absent gateway fails
// closed as unsupported_platform.
func analyzeDriverPackageCategory(ctx context.Context, opts Options, category string) ServicingOperation {
	op := ServicingOperation{
		Category:      category,
		PlannedAction: PlannedActionInvokeWindowsServicing,
		Capability:    ServicingCapabilityAnalyzeDriverStore,
	}
	if ctx != nil && ctx.Err() != nil {
		op.Outcome = ServicingOutcomeCanceled
		op.Reason = ServicingReasonContextCanceled
		return op
	}
	if opts.ServicingGateway == nil {
		op.Outcome = ServicingOutcomeSkipped
		op.Reason = ServicingReasonUnsupportedPlatform
		return op
	}
	return applyDriverStoreAnalysis(op, opts.ServicingGateway.AnalyzeDriverStore(ctx))
}

// applyDriverStoreAnalysis maps an inventory result onto the operation record
// with a fail-closed guard: a ready outcome must carry at least one candidate
// with a valid published name, and duplicate or invalid entries are rejected as
// uninterpretable rather than dropped.
func applyDriverStoreAnalysis(op ServicingOperation, res DriverStoreAnalysisResult) ServicingOperation {
	switch res.Outcome {
	case ServicingOutcomeReady:
		packages, ok := normalizeDriverCandidates(res.Packages)
		if !ok || len(packages) == 0 {
			op.Outcome = ServicingOutcomeFailed
			op.Reason = ServicingReasonAnalysisOutputInvalid
			return op
		}
		op.Outcome = ServicingOutcomeReady
		op.DriverPackages = packages
		op.ReclaimablePackages = len(packages)
		op.CleanupRecommended = true
		op.PackageBytes = driverPackageBytes(packages, "")
	case ServicingOutcomeNoWork:
		op.Outcome = ServicingOutcomeNoWork
	case ServicingOutcomeSkipped, ServicingOutcomeFailed, ServicingOutcomeCanceled:
		op.Outcome = res.Outcome
		op.Reason = normalizeServicingReason(res.Outcome, res.Reason)
	default:
		op.Outcome = ServicingOutcomeFailed
		op.Reason = ServicingReasonHelperFailed
	}
	return op
}

func normalizeDriverCandidates(packages []ServicingDriverPackage) ([]ServicingDriverPackage, bool) {
	out := make([]ServicingDriverPackage, 0, len(packages))
	seen := make(map[string]struct{}, len(packages))
	for _, pkg := range packages {
		if !driverstore.ValidPublishedName(pkg.PublishedName) || pkg.Bytes < 0 {
			return nil, false
		}
		key := strings.ToLower(pkg.PublishedName)
		if _, dup := seen[key]; dup {
			return nil, false
		}
		seen[key] = struct{}{}
		pkg.Outcome = DriverPackageOutcomeCandidate
		out = append(out, pkg)
	}
	return out, true
}

// driverPackageBytes sums package bytes, optionally only for one outcome.
func driverPackageBytes(packages []ServicingDriverPackage, outcome string) *int64 {
	var total int64
	for _, pkg := range packages {
		if outcome == "" || pkg.Outcome == outcome {
			total += pkg.Bytes
		}
	}
	return &total
}

// executeDriverPackageCategory removes confirmed superseded display driver
// packages. Without --allow-servicing it skips without analysis or UAC. It runs
// a fresh non-elevated analysis, bounds the work to the confirmed package set
// when the TUI froze one, and only then asks the elevated helper to remove the
// named packages; the helper re-derives eligibility itself.
func executeDriverPackageCategory(ctx context.Context, opts Options, category string) ServicingOperation {
	op := ServicingOperation{
		Category:      category,
		PlannedAction: PlannedActionInvokeWindowsServicing,
		Capability:    ServicingCapabilityExecuteDriverPackageCleanup,
	}
	if !opts.AllowServicing {
		op.Outcome = ServicingOutcomeSkipped
		op.Reason = ServicingReasonNotAuthorized
		return op
	}
	if ctx != nil && ctx.Err() != nil {
		op.Outcome = ServicingOutcomeCanceled
		op.Reason = ServicingReasonContextCanceled
		return op
	}
	gateway := opts.ServicingGateway
	if gateway == nil {
		op.Outcome = ServicingOutcomeSkipped
		op.Reason = ServicingReasonUnsupportedPlatform
		return op
	}

	analysis := applyDriverStoreAnalysis(ServicingOperation{}, gateway.AnalyzeDriverStore(ctx))
	if analysis.Outcome != ServicingOutcomeReady {
		op.Outcome = analysis.Outcome
		op.Reason = analysis.Reason
		return op
	}
	candidates := confirmedDriverCandidates(analysis.DriverPackages, opts.ConfirmedDriverPackages)
	if len(candidates) == 0 {
		op.Outcome = ServicingOutcomeNoWork
		return op
	}
	names := make([]string, 0, len(candidates))
	for _, pkg := range candidates {
		names = append(names, pkg.PublishedName)
	}
	if err := driverstore.ValidateRequest(names); err != nil {
		op.Outcome = ServicingOutcomeFailed
		op.Reason = ServicingReasonAnalysisOutputInvalid
		return op
	}
	if ctx != nil && ctx.Err() != nil {
		op.Outcome = ServicingOutcomeCanceled
		op.Reason = ServicingReasonContextCanceled
		return op
	}
	res := gateway.ExecuteDriverPackageCleanup(ctx, DriverPackageCleanupRequest{
		Category:   category,
		Capability: ServicingCapabilityExecuteDriverPackageCleanup,
		Packages:   names,
	})
	return applyDriverPackageCleanup(op, candidates, res)
}

// confirmedDriverCandidates bounds fresh candidates to the confirmed set. A nil
// confirmed set (CLI) keeps every fresh candidate; an empty non-nil set keeps
// none.
func confirmedDriverCandidates(fresh []ServicingDriverPackage, confirmed []string) []ServicingDriverPackage {
	if confirmed == nil {
		return fresh
	}
	allowed := make(map[string]struct{}, len(confirmed))
	for _, name := range confirmed {
		allowed[strings.ToLower(name)] = struct{}{}
	}
	out := make([]ServicingDriverPackage, 0, len(fresh))
	for _, pkg := range fresh {
		if _, ok := allowed[strings.ToLower(pkg.PublishedName)]; ok {
			out = append(out, pkg)
		}
	}
	return out
}

// applyDriverPackageCleanup merges the helper's per-package outcomes onto the
// requested candidates and classifies the operation fail-closed: any failed or
// unreported package fails the operation; otherwise at least one removal is
// completed and none is no_work. Packages the helper never attempted keep the
// candidate outcome.
func applyDriverPackageCleanup(op ServicingOperation, candidates []ServicingDriverPackage, res DriverPackageCleanupResult) ServicingOperation {
	op.ReclaimablePackages = len(candidates)
	op.CleanupRecommended = true
	op.CancelRequested = res.CancelRequested
	packages := append([]ServicingDriverPackage(nil), candidates...)

	switch res.Outcome {
	case ServicingOutcomeSkipped, ServicingOutcomeCanceled:
		op.Outcome = res.Outcome
		op.Reason = normalizeServicingReason(res.Outcome, res.Reason)
		op.DriverPackages = packages
		return op
	case ServicingOutcomeCompleted, ServicingOutcomeNoWork, ServicingOutcomeFailed:
	default:
		op.Outcome = ServicingOutcomeFailed
		op.Reason = ServicingReasonHelperFailed
		op.DriverPackages = packages
		return op
	}

	reported := map[string]string{}
	for _, pkg := range res.Packages {
		switch pkg.Outcome {
		case DriverPackageOutcomeRemoved, DriverPackageOutcomeInUse, DriverPackageOutcomeNotEligible, DriverPackageOutcomeFailed:
			reported[strings.ToLower(pkg.PublishedName)] = pkg.Outcome
		}
	}
	if res.Outcome == ServicingOutcomeFailed && len(reported) == 0 {
		// The helper failed before attempting any package.
		op.Outcome = ServicingOutcomeFailed
		op.Reason = normalizeServicingReason(ServicingOutcomeFailed, res.Reason)
		op.DriverPackages = packages
		return op
	}

	removed, failed := 0, 0
	for i := range packages {
		outcome, ok := reported[strings.ToLower(packages[i].PublishedName)]
		if !ok {
			outcome = DriverPackageOutcomeFailed
		}
		packages[i].Outcome = outcome
		switch outcome {
		case DriverPackageOutcomeRemoved:
			removed++
		case DriverPackageOutcomeFailed:
			failed++
		}
	}
	op.DriverPackages = packages
	op.PackageBytes = driverPackageBytes(packages, DriverPackageOutcomeRemoved)
	if removed > 0 {
		op.ObservedFreeBytes = res.ObservedFreeBytes
	}
	switch {
	case failed > 0:
		op.Outcome = ServicingOutcomeFailed
		op.Reason = ServicingReasonCleanupFailed
	case removed > 0:
		op.Outcome = ServicingOutcomeCompleted
	default:
		op.Outcome = ServicingOutcomeNoWork
	}
	return op
}
