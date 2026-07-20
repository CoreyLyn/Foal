package uninstall

import (
	"context"
	"errors"
	"testing"
)

// fakeElevationPort records each RequestElevation call and returns the
// configured granted/err. Tests inject this so no real UAC prompt is ever
// triggered (ADR 0028). calls counts how many times the port was invoked so
// tests can assert elevation is requested at most once per batch.
type fakeElevationPort struct {
	calls   int
	granted bool
	err     error
}

func (f *fakeElevationPort) RequestElevation(context.Context) (bool, error) {
	f.calls++
	if f.err != nil {
		return false, f.err
	}
	return f.granted, nil
}

// TestExecuteRequestsElevationWhenAdminAppSelected verifies that when a
// selected app has an HKLM (machine-wide) source, Execute calls the injectable
// ElevationPort once before processing the batch.
func TestExecuteRequestsElevationWhenAdminAppSelected(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Admin App",
		QuietUninstallCommand: `MsiExec.exe /X{AD1} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	port := &fakeElevationPort{granted: true}
	runner := &fakeUninstallerRunner{results: []UninstallerRunResult{{ExitCode: 0}}}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Admin App"},
		UninstallerRunner: runner,
		ElevationPort:     port,
	})

	if port.calls != 1 {
		t.Fatalf("elevation port calls = %d, want 1 (one request per admin batch)", port.calls)
	}
	if !result.Elevation.Requested {
		t.Fatal("elevation.Requested = false, want true (admin app selected)")
	}
	if !result.Elevation.Granted {
		t.Fatal("elevation.Granted = false, want true (fake granted)")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 (granted elevation proceeds)", len(runner.calls))
	}
}

// TestExecuteDoesNotRequestElevationWhenNoAdminAppSelected verifies that a
// batch of HKCU-only (per-user) apps never triggers an elevation request.
func TestExecuteDoesNotRequestElevationWhenNoAdminAppSelected(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "User App",
		QuietUninstallCommand: `MsiExec.exe /X{U1} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKCU"},
	}})
	port := &fakeElevationPort{granted: false}
	runner := &fakeUninstallerRunner{results: []UninstallerRunResult{{ExitCode: 0}}}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"User App"},
		UninstallerRunner: runner,
		ElevationPort:     port,
	})

	if port.calls != 0 {
		t.Fatalf("elevation port calls = %d, want 0 (no admin app -> no request)", port.calls)
	}
	if result.Elevation.Requested {
		t.Fatal("elevation.Requested = true, want false (no admin app)")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 (non-admin app proceeds without elevation)", len(runner.calls))
	}
}

// TestExecuteSkipsAdminAppWhenElevationDenied verifies that when the
// ElevationPort denies, an admin-required app is skipped with the stable
// SkipElevationRequiredNotGranted reason and its uninstaller is never invoked.
// The skip reason must remain stable so consumers can rely on the code.
func TestExecuteSkipsAdminAppWhenElevationDenied(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Admin App",
		QuietUninstallCommand: `MsiExec.exe /X{AD2} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	port := &fakeElevationPort{granted: false}
	runner := &fakeUninstallerRunner{}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Admin App"},
		UninstallerRunner: runner,
		ElevationPort:     port,
	})

	if port.calls != 1 {
		t.Fatalf("elevation port calls = %d, want 1", port.calls)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0 (denied elevation must not run uninstaller)", len(runner.calls))
	}
	if len(result.Applications) != 1 {
		t.Fatalf("applications = %d, want 1", len(result.Applications))
	}
	app := result.Applications[0]
	if app.Result != ResultSkipped {
		t.Fatalf("result = %q, want %q", app.Result, ResultSkipped)
	}
	if app.SkippedReason != SkipElevationRequiredNotGranted {
		t.Fatalf("skipped reason = %q, want %q", app.SkippedReason, SkipElevationRequiredNotGranted)
	}
	if !app.RequiresAdmin {
		t.Fatal("RequiresAdmin = false, want true (HKLM app)")
	}
	if result.Elevation.Requested != true || result.Elevation.Granted != false {
		t.Fatalf("elevation = %#v, want requested=true granted=false", result.Elevation)
	}
	if result.Totals.SkippedCount != 1 || result.Totals.UninstalledCount != 0 {
		t.Fatalf("totals = %#v, want 1 skipped 0 uninstalled", result.Totals)
	}
}

// TestExecuteSkipsAdminAppWhenElevationPortReturnsError verifies that a port
// error is treated as elevation unavailable: admin apps skip with the stable
// reason and Foal does not crash or silently change outcomes.
func TestExecuteSkipsAdminAppWhenElevationPortReturnsError(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Admin App",
		QuietUninstallCommand: `MsiExec.exe /X{AD3} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	port := &fakeElevationPort{err: errors.New("UAC service unavailable")}
	runner := &fakeUninstallerRunner{}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Admin App"},
		UninstallerRunner: runner,
		ElevationPort:     port,
	})

	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0 (elevation error must not run uninstaller)", len(runner.calls))
	}
	app := result.Applications[0]
	if app.Result != ResultSkipped || app.SkippedReason != SkipElevationRequiredNotGranted {
		t.Fatalf("app = %#v, want skipped/%q", app, SkipElevationRequiredNotGranted)
	}
	if !result.Elevation.Requested || result.Elevation.Granted {
		t.Fatalf("elevation = %#v, want requested=true granted=false", result.Elevation)
	}
}

// TestExecuteProceedsAdminAppWhenElevationGranted verifies that when the port
// grants, an admin-required app's uninstaller runs normally and the outcome is
// recorded as uninstalled on success.
func TestExecuteProceedsAdminAppWhenElevationGranted(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Admin App",
		QuietUninstallCommand: `MsiExec.exe /X{AD4} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	port := &fakeElevationPort{granted: true}
	runner := &fakeUninstallerRunner{results: []UninstallerRunResult{{ExitCode: 0}}}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Admin App"},
		UninstallerRunner: runner,
		ElevationPort:     port,
	})

	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 (granted elevation proceeds)", len(runner.calls))
	}
	app := result.Applications[0]
	if app.Result != ResultUninstalled {
		t.Fatalf("result = %q, want %q", app.Result, ResultUninstalled)
	}
	if !app.RequiresAdmin {
		t.Fatal("RequiresAdmin = false, want true")
	}
	if !result.Elevation.Granted {
		t.Fatal("elevation.Granted = false, want true")
	}
}

// TestExecuteNonAdminAppProceedsWhenElevationDenied verifies that a non-admin
// (HKCU) app proceeds even when the port denies elevation. Elevation denial
// only affects admin-required apps; batch isolation is preserved.
func TestExecuteNonAdminAppProceedsWhenElevationDenied(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "User App",
		QuietUninstallCommand: `MsiExec.exe /X{U2} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKCU"},
	}})
	port := &fakeElevationPort{granted: false}
	runner := &fakeUninstallerRunner{results: []UninstallerRunResult{{ExitCode: 0}}}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"User App"},
		UninstallerRunner: runner,
		ElevationPort:     port,
	})

	// Non-admin app: port is NOT called (no admin app in the batch).
	if port.calls != 0 {
		t.Fatalf("elevation port calls = %d, want 0 (non-admin batch)", port.calls)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 (non-admin app proceeds)", len(runner.calls))
	}
	app := result.Applications[0]
	if app.Result != ResultUninstalled {
		t.Fatalf("result = %q, want %q", app.Result, ResultUninstalled)
	}
	if app.RequiresAdmin {
		t.Fatal("RequiresAdmin = true, want false (HKCU app)")
	}
}

// TestExecuteNilElevationPortProceedsWithAdminApps verifies the production
// default: when no port is wired, Foal does not request UAC itself and
// proceeds with current process privileges. This preserves the ability to
// uninstall machine-wide apps when Foal is already elevated, and keeps the
// seam injectable for tests and a future real UAC implementation.
func TestExecuteNilElevationPortProceedsWithAdminApps(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Admin App",
		QuietUninstallCommand: `MsiExec.exe /X{AD5} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{results: []UninstallerRunResult{{ExitCode: 0}}}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Admin App"},
		UninstallerRunner: runner,
		// ElevationPort intentionally nil: production default.
	})

	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 (nil port proceeds with admin apps)", len(runner.calls))
	}
	app := result.Applications[0]
	if app.Result != ResultUninstalled {
		t.Fatalf("result = %q, want %q", app.Result, ResultUninstalled)
	}
	if !result.Elevation.Requested {
		t.Fatal("elevation.Requested = false, want true (admin app selected)")
	}
	if !result.Elevation.Granted {
		t.Fatal("elevation.Granted = false, want true (nil port proceeds)")
	}
}

// TestExecuteElevationRequestedOncePerBatch verifies that even with multiple
// admin-required apps, the port is called at most once (UAC is per-batch, not
// per-app), and a mix of admin and non-admin apps is handled correctly.
func TestExecuteElevationRequestedOncePerBatch(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{
		{
			Name:                  "Admin One",
			QuietUninstallCommand: `MsiExec.exe /X{A1} /qn`,
			Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
		},
		{
			Name:                  "Admin Two",
			QuietUninstallCommand: `MsiExec.exe /X{A2} /qn`,
			Sources:               []string{"windows_registry_uninstall_keys:HKLM32"},
		},
		{
			Name:                  "User One",
			QuietUninstallCommand: `MsiExec.exe /X{U3} /qn`,
			Sources:               []string{"windows_registry_uninstall_keys:HKCU"},
		},
	})
	port := &fakeElevationPort{granted: true}
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{
			{ExitCode: 0}, // Admin One
			{ExitCode: 0}, // Admin Two
			{ExitCode: 0}, // User One
		},
	}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Admin One", "Admin Two", "User One"},
		UninstallerRunner: runner,
		ElevationPort:     port,
	})

	if port.calls != 1 {
		t.Fatalf("elevation port calls = %d, want 1 (one request per batch)", port.calls)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("runner calls = %d, want 3 (all apps proceed after grant)", len(runner.calls))
	}
	if result.Totals.UninstalledCount != 3 {
		t.Fatalf("uninstalled = %d, want 3", result.Totals.UninstalledCount)
	}
}

// TestExecuteElevationDenialIsolatesAdminAndNonAdminApps verifies batch
// isolation under mixed elevation outcomes: when elevation is denied, admin
// apps skip while a non-admin app in the same batch still uninstalls. The
// batch does not abort on the admin skip.
func TestExecuteElevationDenialIsolatesAdminAndNonAdminApps(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{
		{
			Name:                  "Admin App",
			QuietUninstallCommand: `MsiExec.exe /X{A3} /qn`,
			Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
		},
		{
			Name:                  "User App",
			QuietUninstallCommand: `MsiExec.exe /X{U4} /qn`,
			Sources:               []string{"windows_registry_uninstall_keys:HKCU"},
		},
	})
	port := &fakeElevationPort{granted: false}
	runner := &fakeUninstallerRunner{results: []UninstallerRunResult{{ExitCode: 0}}}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Admin App", "User App"},
		UninstallerRunner: runner,
		ElevationPort:     port,
	})

	// stableSelection sorts alphabetically: "Admin App" then "User App".
	if len(result.Applications) != 2 {
		t.Fatalf("applications = %d, want 2 (batch must not abort)", len(result.Applications))
	}
	admin := result.Applications[0]
	user := result.Applications[1]
	if admin.Name != "Admin App" || admin.Result != ResultSkipped || admin.SkippedReason != SkipElevationRequiredNotGranted {
		t.Fatalf("admin app = %#v, want skipped/%q", admin, SkipElevationRequiredNotGranted)
	}
	if user.Name != "User App" || user.Result != ResultUninstalled {
		t.Fatalf("user app = %#v, want uninstalled (non-admin proceeds)", user)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 (only the non-admin app runs)", len(runner.calls))
	}
}

// TestExecuteElevationOutcomeOmittedWhenNoAdminApp verifies the result
// elevation outcome reports Requested=false (and thus no elevation is
// rendered) when the batch contains no admin-required apps.
func TestExecuteElevationOutcomeOmittedWhenNoAdminApp(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "User App",
		QuietUninstallCommand: `MsiExec.exe /X{U5} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKCU"},
	}})
	runner := &fakeUninstallerRunner{results: []UninstallerRunResult{{ExitCode: 0}}}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"User App"},
		UninstallerRunner: runner,
	})

	if result.Elevation.Requested {
		t.Fatal("elevation.Requested = true, want false (no admin app)")
	}
}

// TestExecuteDoesNotRequestElevationForHardExclusionAdminApp verifies that
// an admin-required app that is a hard exclusion (Foal itself, HKLM source)
// does NOT trigger an elevation request, because Foal never executes it.
// Elevation is requested only for apps Foal would actually run.
func TestExecuteDoesNotRequestElevationForHardExclusionAdminApp(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Foal",
		QuietUninstallCommand: `MsiExec.exe /X{FOAL} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	port := &fakeElevationPort{granted: true}
	runner := &fakeUninstallerRunner{}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Foal"},
		UninstallerRunner: runner,
		ElevationPort:     port,
	})

	if port.calls != 0 {
		t.Fatalf("elevation port calls = %d, want 0 (hard exclusion never executes)", port.calls)
	}
	if result.Elevation.Requested {
		t.Fatal("elevation.Requested = true, want false (hard exclusion does not need elevation)")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0 (hard exclusion never runs)", len(runner.calls))
	}
}
