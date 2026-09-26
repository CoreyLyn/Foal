//go:build windows

package servicing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/driverstore"
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

func displayPackage(published, date, version string, inUse bool) driverstore.Package {
	return driverstore.Package{
		PublishedName: published,
		OriginalName:  "nv_dispi.inf",
		Provider:      "NVIDIA",
		ClassGUID:     driverstore.DisplayClassGUID,
		DriverDate:    date,
		DriverVersion: version,
		InUse:         inUse,
	}
}

// fakeDriverDeps never touches the real driver store. inventories are returned
// in order, one per inventory call (the last repeats).
type fakeDriverDeps struct {
	elevated       bool
	inventories    []driverstore.Inventory
	inventoryErrAt int // 1-based call that fails; 0 never
	removeOutcomes map[string]driverstore.RemoveOutcome
	free           []uint64
	inventoryCalls int
	removed        []string
	freeCalls      int
}

func (f *fakeDriverDeps) deps() driverCleanupDeps {
	return driverCleanupDeps{
		elevated: func() bool { return f.elevated },
		inventory: func(context.Context) (driverstore.Inventory, error) {
			f.inventoryCalls++
			if f.inventoryErrAt != 0 && f.inventoryCalls >= f.inventoryErrAt {
				return driverstore.Inventory{}, errors.New("inventory failed")
			}
			if len(f.inventories) == 0 {
				return driverstore.Inventory{}, nil
			}
			i := f.inventoryCalls - 1
			if i >= len(f.inventories) {
				i = len(f.inventories) - 1
			}
			return f.inventories[i], nil
		},
		remove: func(name string) driverstore.RemoveOutcome {
			f.removed = append(f.removed, name)
			if outcome, ok := f.removeOutcomes[name]; ok {
				return outcome
			}
			return driverstore.RemoveOutcomeRemoved
		},
		freeBytes: func() (uint64, bool) {
			f.freeCalls++
			if len(f.free) < f.freeCalls {
				return 0, false
			}
			return f.free[f.freeCalls-1], true
		},
	}
}

func driverOutcomes(res clean.DriverPackageCleanupResult) string {
	outcomes := make([]string, 0, len(res.Packages))
	for _, pkg := range res.Packages {
		outcomes = append(outcomes, pkg.PublishedName+"="+pkg.Outcome)
	}
	return strings.Join(outcomes, ",")
}

var threeVersionInventory = driverstore.Inventory{Packages: []driverstore.Package{
	displayPackage("oem10.inf", "01/01/2024", "31.0.15.1000", false),
	displayPackage("oem11.inf", "01/01/2025", "32.0.15.1000", false),
	displayPackage("oem12.inf", "06/01/2025", "32.0.15.2000", false),
}}

func TestRunDriverPackageCleanupRejectsInvalidRequestBeforeAnything(t *testing.T) {
	valid := driverstore.IdentityOf(threeVersionInventory.Packages[0])
	pathLike := valid
	pathLike.PublishedName = `..\oem10.inf`
	duplicate := valid
	duplicate.PublishedName = "OEM10.INF"
	for _, requested := range [][]driverstore.Identity{nil, {pathLike}, {valid, duplicate}} {
		fake := &fakeDriverDeps{elevated: true, inventories: []driverstore.Inventory{threeVersionInventory}}
		res := runDriverPackageCleanupWith(fake.deps(), requested)
		if res.Outcome != clean.ServicingOutcomeFailed || res.Reason != clean.ServicingReasonHelperFailed || len(res.Packages) != 0 {
			t.Fatalf("runDriverPackageCleanupWith(%v) = %#v", requested, res)
		}
		if fake.inventoryCalls != 0 || len(fake.removed) != 0 {
			t.Fatalf("invalid request reached inventory=%d removals=%v", fake.inventoryCalls, fake.removed)
		}
	}
}

func TestRunDriverPackageCleanupRequiresElevation(t *testing.T) {
	fake := &fakeDriverDeps{elevated: false, inventories: []driverstore.Inventory{threeVersionInventory}}
	res := runDriverPackageCleanupWith(fake.deps(), []driverstore.Identity{driverstore.IdentityOf(threeVersionInventory.Packages[0])})
	if res.Outcome != clean.ServicingOutcomeFailed || res.Reason != clean.ServicingReasonElevationFailed || len(res.Packages) != 0 {
		t.Fatalf("unelevated result = %#v", res)
	}
	if fake.inventoryCalls != 0 || len(fake.removed) != 0 {
		t.Fatalf("unelevated helper touched inventory=%d removals=%v", fake.inventoryCalls, fake.removed)
	}
}

func TestRunDriverPackageCleanupRevalidatesEachPackageIdentity(t *testing.T) {
	reused := driverstore.IdentityOf(threeVersionInventory.Packages[1])
	reused.DriverVersion = "32.0.15.9999" // oem11.inf now names a different package
	fake := &fakeDriverDeps{
		elevated:    true,
		inventories: []driverstore.Inventory{threeVersionInventory},
		free:        []uint64{100, 150},
	}
	res := runDriverPackageCleanupWith(fake.deps(), []driverstore.Identity{
		driverstore.IdentityOf(threeVersionInventory.Packages[0]),
		reused,
		driverstore.IdentityOf(threeVersionInventory.Packages[2]), // newest: never superseded
	})
	if got := driverOutcomes(res); got != "oem10.inf=removed,oem11.inf=not_eligible,oem12.inf=not_eligible" {
		t.Fatalf("outcomes = %s", got)
	}
	if strings.Join(fake.removed, ",") != "oem10.inf" || fake.inventoryCalls != 3 {
		t.Fatalf("removals = %v inventory calls = %d, want one fresh inventory per package", fake.removed, fake.inventoryCalls)
	}
	if res.Outcome != clean.ServicingOutcomeCompleted || res.ObservedFreeBytes == nil || *res.ObservedFreeBytes != 50 {
		t.Fatalf("result = %#v", res)
	}
}

func TestRunDriverPackageCleanupStopsAttemptsAfterInventoryFailure(t *testing.T) {
	fake := &fakeDriverDeps{elevated: true, inventories: []driverstore.Inventory{threeVersionInventory}, inventoryErrAt: 2}
	res := runDriverPackageCleanupWith(fake.deps(), []driverstore.Identity{
		driverstore.IdentityOf(threeVersionInventory.Packages[0]),
		driverstore.IdentityOf(threeVersionInventory.Packages[1]),
	})
	if got := driverOutcomes(res); got != "oem10.inf=removed,oem11.inf=candidate" {
		t.Fatalf("outcomes = %s", got)
	}
	if res.Outcome != clean.ServicingOutcomeFailed || res.Reason != clean.ServicingReasonAnalysisFailed || len(fake.removed) != 1 {
		t.Fatalf("result = %#v removals = %v", res, fake.removed)
	}
}

func TestRunDriverPackageCleanupMapsWindowsRefusals(t *testing.T) {
	fake := &fakeDriverDeps{
		elevated:    true,
		inventories: []driverstore.Inventory{threeVersionInventory},
		removeOutcomes: map[string]driverstore.RemoveOutcome{
			"oem10.inf": driverstore.RemoveOutcomeInUse,
			"oem11.inf": driverstore.RemoveOutcomeFailed,
		},
		free: []uint64{100, 100},
	}
	res := runDriverPackageCleanupWith(fake.deps(), []driverstore.Identity{
		driverstore.IdentityOf(threeVersionInventory.Packages[0]),
		driverstore.IdentityOf(threeVersionInventory.Packages[1]),
	})
	if got := driverOutcomes(res); got != "oem10.inf=in_use,oem11.inf=failed" {
		t.Fatalf("outcomes = %s", got)
	}
	if res.Outcome != clean.ServicingOutcomeFailed || res.Reason != clean.ServicingReasonCleanupFailed || res.ObservedFreeBytes != nil {
		t.Fatalf("result = %#v", res)
	}
}

func TestDriverExchangeFailureMarksSentPackagesUnknown(t *testing.T) {
	ids := []driverstore.Identity{driverstore.IdentityOf(threeVersionInventory.Packages[0])}
	sent := driverExchangeFailure(ids, true, true)
	if !sent.RequestSent || !sent.CancelRequested || sent.Outcome != clean.ServicingOutcomeFailed || driverOutcomes(sent) != "oem10.inf=unknown" {
		t.Fatalf("post-send failure = %#v", sent)
	}
	unsent := driverExchangeFailure(ids, false, false)
	if unsent.RequestSent || len(unsent.Packages) != 0 || unsent.Outcome != clean.ServicingOutcomeFailed {
		t.Fatalf("pre-send failure = %#v", unsent)
	}
}

// noHelperLaunch fails the test if the gateway tries to launch the elevated
// helper.
func noHelperLaunch(t *testing.T) {
	t.Helper()
	original := establishHelperSession
	establishHelperSession = func() (*helperSession, string, bool, bool) {
		t.Error("gateway tried to launch the elevated helper")
		return nil, clean.ServicingReasonHelperFailed, false, false
	}
	t.Cleanup(func() { establishHelperSession = original })
}

func TestExecuteDriverPackageCleanupRejectsWrongCapabilityOrPackages(t *testing.T) {
	noHelperLaunch(t)
	gateway := NewGateway()
	valid := clean.ServicingDriverPackage{PublishedName: "oem999999.inf", OriginalName: "nv_dispi.inf", Provider: "NVIDIA", DriverDate: "01/01/2024", DriverVersion: "31.0.15.1000"}
	if res := gateway.ExecuteDriverPackageCleanup(context.Background(), clean.DriverPackageCleanupRequest{
		Capability: clean.ServicingCapabilityExecuteComponentStoreCleanup, Packages: []clean.ServicingDriverPackage{valid},
	}); res.Outcome != clean.ServicingOutcomeFailed || res.RequestSent {
		t.Fatalf("wrong capability = %#v", res)
	}
	pathLike := valid
	pathLike.PublishedName = `C:\x.inf`
	noIdentity := valid
	noIdentity.OriginalName = ""
	for _, pkg := range []clean.ServicingDriverPackage{pathLike, noIdentity} {
		if res := gateway.ExecuteDriverPackageCleanup(context.Background(), clean.DriverPackageCleanupRequest{
			Capability: clean.ServicingCapabilityExecuteDriverPackageCleanup, Packages: []clean.ServicingDriverPackage{pkg},
		}); res.Outcome != clean.ServicingOutcomeFailed || res.RequestSent {
			t.Fatalf("invalid package %#v = %#v", pkg, res)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if res := gateway.ExecuteDriverPackageCleanup(ctx, clean.DriverPackageCleanupRequest{
		Capability: clean.ServicingCapabilityExecuteDriverPackageCleanup, Packages: []clean.ServicingDriverPackage{valid},
	}); res.Outcome != clean.ServicingOutcomeCanceled || res.RequestSent {
		t.Fatalf("pre-canceled = %#v, want canceled without elevation", res)
	}
}
