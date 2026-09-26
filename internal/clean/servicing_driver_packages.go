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
// a fresh non-elevated analysis, binds the work to the confirmed packages when
// the TUI froze a set, sends at most driverstore.MaxRequestPackages identities
// (the rest stay candidates for a later run), and only then asks the elevated
// helper to remove them; the helper re-derives eligibility and identity itself
// immediately before each removal.
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
	candidates, ineligible := confirmedDriverCandidates(analysis.DriverPackages, opts.ConfirmedDriverPackages)
	var deferred []ServicingDriverPackage
	if len(candidates) > driverstore.MaxRequestPackages {
		deferred = candidates[driverstore.MaxRequestPackages:]
		candidates = candidates[:driverstore.MaxRequestPackages]
	}
	if len(candidates) == 0 {
		op.Outcome = ServicingOutcomeNoWork
		op.DriverPackages = ineligible
		return op
	}
	op.ReclaimablePackages = len(candidates) + len(deferred)
	op.CleanupRecommended = true
	if err := driverstore.ValidateIdentities(driverIdentities(candidates)); err != nil {
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
		Packages:   append([]ServicingDriverPackage(nil), candidates...),
	})
	op = applyDriverPackageCleanup(op, candidates, res)
	op.DriverPackages = append(append(op.DriverPackages, deferred...), ineligible...)
	return op
}

// driverIdentity is the identity a removal request binds for one package.
func driverIdentity(pkg ServicingDriverPackage) driverstore.Identity {
	return driverstore.Identity{
		PublishedName: pkg.PublishedName,
		OriginalName:  pkg.OriginalName,
		Provider:      pkg.Provider,
		DriverDate:    pkg.DriverDate,
		DriverVersion: pkg.DriverVersion,
	}
}

// driverIdentities projects packages onto the identities a removal request
// binds.
func driverIdentities(packages []ServicingDriverPackage) []driverstore.Identity {
	ids := make([]driverstore.Identity, 0, len(packages))
	for _, pkg := range packages {
		ids = append(ids, driverIdentity(pkg))
	}
	return ids
}

// sameDriverIdentity reports whether two records describe the same package.
func sameDriverIdentity(a, b ServicingDriverPackage) bool {
	return driverIdentity(a).Matches(driverstore.Package{
		PublishedName: b.PublishedName,
		OriginalName:  b.OriginalName,
		Provider:      b.Provider,
		DriverDate:    b.DriverDate,
		DriverVersion: b.DriverVersion,
	})
}

// confirmedDriverCandidates binds fresh candidates to the confirmed set. A nil
// confirmed set (CLI) keeps every fresh candidate. Otherwise a fresh candidate
// is kept only when a confirmed entry has the same published name and
// identity; confirmed packages without such a fresh match are returned as
// not_eligible, and fresh candidates that were never confirmed are ignored.
func confirmedDriverCandidates(fresh, confirmed []ServicingDriverPackage) (kept, ineligible []ServicingDriverPackage) {
	if confirmed == nil {
		return fresh, nil
	}
	byName := make(map[string]ServicingDriverPackage, len(fresh))
	for _, pkg := range fresh {
		byName[strings.ToLower(pkg.PublishedName)] = pkg
	}
	kept = make([]ServicingDriverPackage, 0, len(confirmed))
	seen := make(map[string]bool, len(confirmed))
	for _, want := range confirmed {
		key := strings.ToLower(want.PublishedName)
		if seen[key] {
			continue
		}
		seen[key] = true
		if pkg, ok := byName[key]; ok && sameDriverIdentity(want, pkg) {
			kept = append(kept, pkg)
			continue
		}
		want.Outcome = DriverPackageOutcomeNotEligible
		ineligible = append(ineligible, want)
	}
	return kept, ineligible
}

// validReportedDriverOutcome reports a per-package outcome a removal result
// may carry.
func validReportedDriverOutcome(outcome string) bool {
	switch outcome {
	case DriverPackageOutcomeCandidate, DriverPackageOutcomeRemoved, DriverPackageOutcomeInUse,
		DriverPackageOutcomeNotEligible, DriverPackageOutcomeFailed, DriverPackageOutcomeUnknown:
		return true
	default:
		return false
	}
}

// applyDriverPackageCleanup merges the removal result onto the requested
// packages fail-closed. Before the request reached the helper nothing was
// attempted, so every package stays a candidate. Afterwards removal may have
// begun: every requested package needs exactly one report with a known
// outcome, the result may only be completed, no_work, or failed and must agree
// with the reports, and an observation must be non-negative and accompany a
// removal. Any violation fails the operation as a helper failure; requested
// packages without a trustworthy report become unknown, and reported
// removals — including names outside the request — stay on the record.
func applyDriverPackageCleanup(op ServicingOperation, requested []ServicingDriverPackage, res DriverPackageCleanupResult) ServicingOperation {
	op.CancelRequested = res.CancelRequested
	packages := append([]ServicingDriverPackage(nil), requested...)
	op.DriverPackages = packages
	if !res.RequestSent {
		switch res.Outcome {
		case ServicingOutcomeSkipped, ServicingOutcomeCanceled, ServicingOutcomeFailed:
			op.Outcome = res.Outcome
			op.Reason = normalizeServicingReason(res.Outcome, res.Reason)
		default:
			op.Outcome = ServicingOutcomeFailed
			op.Reason = ServicingReasonHelperFailed
		}
		return op
	}
	switch res.Outcome {
	case ServicingOutcomeCompleted, ServicingOutcomeNoWork, ServicingOutcomeFailed:
	default:
		op.Outcome = ServicingOutcomeFailed
		op.Reason = ServicingReasonHelperFailed
		for i := range packages {
			packages[i].Outcome = DriverPackageOutcomeUnknown
		}
		return op
	}
	if res.Outcome == ServicingOutcomeFailed && len(res.Packages) == 0 {
		// The helper refused before attempting any package.
		op.Outcome = ServicingOutcomeFailed
		op.Reason = normalizeServicingReason(ServicingOutcomeFailed, res.Reason)
		return op
	}

	valid := true
	index := make(map[string]int, len(packages))
	for i, pkg := range packages {
		index[strings.ToLower(pkg.PublishedName)] = i
	}
	reported := make([]string, len(packages))
	var extras []ServicingDriverPackage
	for _, pkg := range res.Packages {
		if !validReportedDriverOutcome(pkg.Outcome) {
			valid = false
			continue
		}
		i, ok := index[strings.ToLower(pkg.PublishedName)]
		if !ok {
			valid = false
			if driverstore.ValidPublishedName(pkg.PublishedName) {
				extras = append(extras, ServicingDriverPackage{PublishedName: pkg.PublishedName, Outcome: pkg.Outcome})
			}
			continue
		}
		if reported[i] != "" {
			valid = false
			continue
		}
		reported[i] = pkg.Outcome
	}

	removed, unfinished := 0, 0
	for i := range packages {
		outcome := reported[i]
		if outcome == "" {
			valid = false
			outcome = DriverPackageOutcomeUnknown
		}
		packages[i].Outcome = outcome
		switch outcome {
		case DriverPackageOutcomeRemoved:
			removed++
		case DriverPackageOutcomeFailed, DriverPackageOutcomeCandidate, DriverPackageOutcomeUnknown:
			unfinished++
		}
	}
	expected := ServicingOutcomeNoWork
	switch {
	case unfinished > 0:
		expected = ServicingOutcomeFailed
	case removed > 0:
		expected = ServicingOutcomeCompleted
	}
	if res.Outcome != expected {
		valid = false
	}
	if res.ObservedFreeBytes != nil && (*res.ObservedFreeBytes < 0 || removed == 0) {
		valid = false
	}
	op.DriverPackages = append(packages, extras...)
	op.PackageBytes = driverPackageBytes(packages, DriverPackageOutcomeRemoved)
	if !valid {
		op.Outcome = ServicingOutcomeFailed
		op.Reason = ServicingReasonHelperFailed
		return op
	}
	if removed > 0 {
		op.ObservedFreeBytes = res.ObservedFreeBytes
	}
	op.Outcome = expected
	if expected == ServicingOutcomeFailed {
		op.Reason = ServicingReasonCleanupFailed
		if isKnownServicingReason(res.Reason) {
			op.Reason = res.Reason
		}
	}
	return op
}
