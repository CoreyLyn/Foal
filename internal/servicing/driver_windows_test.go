//go:build windows

package servicing

import (
	"context"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// TestAnalyzeDriverStoreIsReadOnlyAndWellFormed runs the real non-elevated
// inventory. It never removes anything.
func TestAnalyzeDriverStoreIsReadOnlyAndWellFormed(t *testing.T) {
	res := NewGateway().AnalyzeDriverStore(context.Background())
	switch res.Outcome {
	case clean.ServicingOutcomeReady:
		for _, pkg := range res.Packages {
			if pkg.Outcome != clean.DriverPackageOutcomeCandidate || pkg.PublishedName == "" {
				t.Fatalf("candidate = %#v", pkg)
			}
		}
	case clean.ServicingOutcomeNoWork:
	default:
		t.Fatalf("analysis = %#v, want ready or no_work", res)
	}
}

// TestRunDriverPackageCleanupRejectsInvalidRequestBeforeInventory proves an
// invalid request fails before the helper inventories or removes anything.
func TestRunDriverPackageCleanupRejectsInvalidRequestBeforeInventory(t *testing.T) {
	for _, packages := range [][]string{nil, {`..\oem1.inf`}, {"oem1.inf", "OEM1.inf"}} {
		res := runDriverPackageCleanup(packages)
		if res.Outcome != clean.ServicingOutcomeFailed || res.Reason != clean.ServicingReasonHelperFailed || len(res.Packages) != 0 {
			t.Fatalf("runDriverPackageCleanup(%v) = %#v", packages, res)
		}
	}
}

func TestExecuteDriverPackageCleanupRejectsWrongCapabilityOrPackages(t *testing.T) {
	gateway := NewGateway()
	if res := gateway.ExecuteDriverPackageCleanup(context.Background(), clean.DriverPackageCleanupRequest{
		Capability: clean.ServicingCapabilityExecuteComponentStoreCleanup, Packages: []string{"oem1.inf"},
	}); res.Outcome != clean.ServicingOutcomeFailed {
		t.Fatalf("wrong capability = %#v", res)
	}
	if res := gateway.ExecuteDriverPackageCleanup(context.Background(), clean.DriverPackageCleanupRequest{
		Capability: clean.ServicingCapabilityExecuteDriverPackageCleanup, Packages: []string{`C:\x.inf`},
	}); res.Outcome != clean.ServicingOutcomeFailed {
		t.Fatalf("invalid packages = %#v", res)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if res := gateway.ExecuteDriverPackageCleanup(ctx, clean.DriverPackageCleanupRequest{
		Capability: clean.ServicingCapabilityExecuteDriverPackageCleanup, Packages: []string{"oem1.inf"},
	}); res.Outcome != clean.ServicingOutcomeCanceled {
		t.Fatalf("pre-canceled = %#v, want canceled without elevation", res)
	}
}
