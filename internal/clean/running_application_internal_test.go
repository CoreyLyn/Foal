package clean

import (
	"context"
	"errors"
	"testing"
)

func TestDetectSupportedBrowserApplicationsClassifiesRunningIdleAndUnknown(t *testing.T) {
	running := detectSupportedBrowserApplications(context.Background(), func(context.Context) processSnapshot {
		return processSnapshot{Names: []string{"notepad.exe", "chrome.exe", "msedge.exe", "firefox.exe"}}
	}, func(context.Context) serviceSnapshot {
		return serviceSnapshot{}
	})
	if running[0].Application != ApplicationGoogleChrome || running[0].State != RunningApplicationStateRunning {
		t.Fatalf("Chrome state = %#v, want running", running[0])
	}
	if running[1].Application != ApplicationMicrosoftEdge || running[1].State != RunningApplicationStateRunning {
		t.Fatalf("Edge state = %#v, want running", running[1])
	}
	if running[2].Application != ApplicationMozillaFirefox || running[2].State != RunningApplicationStateRunning {
		t.Fatalf("Firefox state = %#v, want running", running[2])
	}

	idle := detectSupportedBrowserApplications(context.Background(), func(context.Context) processSnapshot {
		return processSnapshot{Names: []string{"notepad.exe"}}
	}, func(context.Context) serviceSnapshot {
		return serviceSnapshot{}
	})
	if idle[0].State != RunningApplicationStateIdle || idle[1].State != RunningApplicationStateIdle || idle[2].State != RunningApplicationStateIdle {
		t.Fatalf("idle states = %#v, want all browsers idle", idle)
	}

	unknownProcess := detectSupportedBrowserApplications(context.Background(), func(context.Context) processSnapshot {
		return processSnapshot{Err: errors.New("snapshot failed")}
	}, func(context.Context) serviceSnapshot {
		return serviceSnapshot{}
	})
	if len(unknownProcess) != 3 {
		t.Fatalf("unknown states = %#v, want Chrome/Edge/Firefox", unknownProcess)
	}
	if unknownProcess[0].State != RunningApplicationStateUnknown || unknownProcess[1].State != RunningApplicationStateUnknown || unknownProcess[2].State != RunningApplicationStateUnknown {
		t.Fatalf("unknown states = %#v, want all unknown", unknownProcess)
	}
	if unknownProcess[0].Message == "" || unknownProcess[1].Message == "" || unknownProcess[2].Message == "" {
		t.Fatalf("unknown states = %#v, want diagnostic messages", unknownProcess)
	}

	unknownService := detectSupportedBrowserApplications(context.Background(), func(context.Context) processSnapshot {
		return processSnapshot{Names: []string{"notepad.exe"}}
	}, func(context.Context) serviceSnapshot {
		return serviceSnapshot{Err: errors.New("service snapshot failed")}
	})
	if len(unknownService) != 3 {
		t.Fatalf("unknown states = %#v, want Chrome/Edge/Firefox", unknownService)
	}
	if unknownService[0].State != RunningApplicationStateUnknown || unknownService[1].State != RunningApplicationStateUnknown || unknownService[2].State != RunningApplicationStateUnknown {
		t.Fatalf("unknown states = %#v, want all unknown", unknownService)
	}
}

func TestClassifyApplicationWithServices(t *testing.T) {
	// Only processes running: application runs
	state := classifyApplication("test-app", []string{"app.exe"}, []string{"AppService"}, []string{"app.exe"}, nil)
	if state.State != RunningApplicationStateRunning {
		t.Fatalf("state = %#v, want running (process detected)", state)
	}

	// Only services running: application runs
	state = classifyApplication("test-app", []string{"app.exe"}, []string{"AppService"}, nil, []string{"AppService"})
	if state.State != RunningApplicationStateRunning {
		t.Fatalf("state = %#v, want running (service detected)", state)
	}

	// Neither running: idle
	state = classifyApplication("test-app", []string{"app.exe"}, []string{"AppService"}, []string{"notepad.exe"}, []string{"OtherService"})
	if state.State != RunningApplicationStateIdle {
		t.Fatalf("state = %#v, want idle", state)
	}

	// Service name comparison is case-insensitive
	state = classifyApplication("test-app", nil, []string{"AppService"}, nil, []string{"appservice"})
	if state.State != RunningApplicationStateRunning {
		t.Fatalf("state = %#v, want running (case-insensitive service match)", state)
	}
}

func TestClassifyBrowserWithServices(t *testing.T) {
	// Only process running: browser runs
	state := classifyBrowser("test-browser", "browser.exe", []string{"BrowserService"}, []string{"browser.exe"}, nil)
	if state.State != RunningApplicationStateRunning {
		t.Fatalf("state = %#v, want running (process detected)", state)
	}

	// Only service running: browser runs
	state = classifyBrowser("test-browser", "browser.exe", []string{"BrowserService"}, nil, []string{"BrowserService"})
	if state.State != RunningApplicationStateRunning {
		t.Fatalf("state = %#v, want running (service detected)", state)
	}

	// Neither running: idle
	state = classifyBrowser("test-browser", "browser.exe", []string{"BrowserService"}, []string{"notepad.exe"}, []string{"OtherService"})
	if state.State != RunningApplicationStateIdle {
		t.Fatalf("state = %#v, want idle", state)
	}
}

func TestDetectSupportedApplicationsUsesRegisteredDeveloperTools(t *testing.T) {
	states := detectSupportedApplications(context.Background(), func(context.Context) processSnapshot {
		return processSnapshot{Names: []string{"go.exe", "cargo.exe", "notepad.exe"}}
	}, func(context.Context) serviceSnapshot {
		return serviceSnapshot{}
	})
	// Browsers first, then developer applications, then application-cache apps.
	wantOrder := []string{
		ApplicationGoogleChrome,
		ApplicationMicrosoftEdge,
		ApplicationMozillaFirefox,
		ApplicationGo,
		ApplicationCargo,
		ApplicationDotNet,
		ApplicationNuGet,
		ApplicationNode,
		ApplicationPython,
		ApplicationUV,
		ApplicationBun,
		ApplicationIntelliJIDEA,
		ApplicationPyCharm,
		ApplicationWebStorm,
		ApplicationPhpStorm,
		ApplicationRubyMine,
		ApplicationCLion,
		ApplicationDataGrip,
		ApplicationDataSpell,
		ApplicationGoLand,
		ApplicationRustRover,
		ApplicationAqua,
		ApplicationMPS,
		ApplicationWriterside,
		ApplicationRider,
		ApplicationVisualStudio,
		ApplicationGrokBuild,
		ApplicationVisualStudioCode,
		ApplicationCursor,
		ApplicationVisualStudioCodeInsiders,
		ApplicationVSCodium,
		ApplicationWindsurf,
		ApplicationTrae,
		ApplicationObsidian,
		ApplicationVRChat,
	}
	if len(states) != len(wantOrder) {
		t.Fatalf("states = %#v, want %d entries", states, len(wantOrder))
	}
	for i, app := range wantOrder {
		if states[i].Application != app {
			t.Fatalf("states[%d].Application = %q, want %q", i, states[i].Application, app)
		}
	}
	if states[3].State != RunningApplicationStateRunning {
		t.Fatalf("go state = %#v, want running", states[3])
	}
	if states[4].State != RunningApplicationStateRunning {
		t.Fatalf("cargo state = %#v, want running", states[4])
	}
	if states[5].State != RunningApplicationStateIdle {
		t.Fatalf("dotnet state = %#v, want idle", states[5])
	}
}

func TestDetectSupportedApplicationsWithService(t *testing.T) {
	// Save original definitions and restore after test
	originalDevApps := developerApplicationDefinitions
	originalAppCacheApps := applicationCacheApplicationDefinitions
	defer func() {
		developerApplicationDefinitions = originalDevApps
		applicationCacheApplicationDefinitions = originalAppCacheApps
	}()

	// Add a test app with a service
	developerApplicationDefinitions = []supportedApplicationDefinition{
		{id: "test-app", displayName: "Test App", executables: []string{"app.exe"}, services: []string{"TestService"}},
	}
	applicationCacheApplicationDefinitions = nil

	// App service is running
	states := detectSupportedApplications(context.Background(), func(context.Context) processSnapshot {
		return processSnapshot{Names: []string{"notepad.exe"}}
	}, func(context.Context) serviceSnapshot {
		return serviceSnapshot{NonStopped: []string{"TestService"}}
	})
	if len(states) != 4 { // 3 browsers + 1 test app
		t.Fatalf("states = %#v, want 4 entries", states)
	}
	if states[3].Application != "test-app" || states[3].State != RunningApplicationStateRunning {
		t.Fatalf("test-app state = %#v, want running (service detected)", states[3])
	}
}

func TestClassifyProcessByExecutablesSupportsMultipleNames(t *testing.T) {
	// uv/uvx: one logical application, either executable means running.
	executables := []string{"uv.exe", "uvx.exe"}
	if got := classifyProcessByExecutables(ApplicationUV, executables, []string{"other.exe", "uvx.exe"}); got.State != RunningApplicationStateRunning {
		t.Fatalf("uvx.exe state = %#v, want running", got)
	}
	if got := classifyProcessByExecutables(ApplicationUV, executables, []string{"UV.EXE"}); got.State != RunningApplicationStateRunning {
		t.Fatalf("case-insensitive uv.exe state = %#v, want running", got)
	}
	if got := classifyProcessByExecutables(ApplicationUV, executables, []string{"notepad.exe"}); got.State != RunningApplicationStateIdle {
		t.Fatalf("idle state = %#v, want idle", got)
	}
}

func TestDetectSupportedApplicationsClassifiesCursorCaseInsensitive(t *testing.T) {
	states := detectSupportedApplications(context.Background(), func(context.Context) processSnapshot {
		return processSnapshot{Names: []string{"CURSOR.EXE", "notepad.exe"}}
	}, func(context.Context) serviceSnapshot {
		return serviceSnapshot{}
	})
	var cursorState RunningApplicationState
	found := false
	for _, state := range states {
		if state.Application == ApplicationCursor {
			cursorState = state
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected cursor application state")
	}
	if cursorState.State != RunningApplicationStateRunning {
		t.Fatalf("Cursor.exe case-insensitive state = %#v, want running", cursorState)
	}
	// Code.exe must remain independent when only Cursor is present.
	for _, state := range states {
		if state.Application == ApplicationVisualStudioCode && state.State != RunningApplicationStateIdle {
			t.Fatalf("VS Code state = %#v, want idle when only Cursor runs", state)
		}
	}
}

func TestDetectSupportedApplicationsTreatsUVxAsUV(t *testing.T) {
	states := detectSupportedApplications(context.Background(), func(context.Context) processSnapshot {
		return processSnapshot{Names: []string{"uvx.exe", "notepad.exe"}}
	}, func(context.Context) serviceSnapshot {
		return serviceSnapshot{}
	})
	var uvState RunningApplicationState
	found := false
	for _, state := range states {
		if state.Application == ApplicationUV {
			uvState = state
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected uv application state")
	}
	if uvState.State != RunningApplicationStateRunning {
		t.Fatalf("uv state with uvx.exe = %#v, want running", uvState)
	}
}

func TestClassifyProcessByExecutablesSupportsBunAndBunx(t *testing.T) {
	executables := []string{"bun.exe", "bunx.exe"}
	if got := classifyProcessByExecutables(ApplicationBun, executables, []string{"other.exe", "bunx.exe"}); got.State != RunningApplicationStateRunning {
		t.Fatalf("bunx.exe state = %#v, want running", got)
	}
	if got := classifyProcessByExecutables(ApplicationBun, executables, []string{"BUN.EXE"}); got.State != RunningApplicationStateRunning {
		t.Fatalf("case-insensitive bun.exe state = %#v, want running", got)
	}
	if got := classifyProcessByExecutables(ApplicationBun, executables, []string{"notepad.exe"}); got.State != RunningApplicationStateIdle {
		t.Fatalf("idle state = %#v, want idle", got)
	}
}

func TestDetectSupportedApplicationsTreatsBunxAsBun(t *testing.T) {
	states := detectSupportedApplications(context.Background(), func(context.Context) processSnapshot {
		return processSnapshot{Names: []string{"bunx.exe", "notepad.exe"}}
	}, func(context.Context) serviceSnapshot {
		return serviceSnapshot{}
	})
	var bunState RunningApplicationState
	found := false
	for _, state := range states {
		if state.Application == ApplicationBun {
			bunState = state
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected bun application state")
	}
	if bunState.State != RunningApplicationStateRunning {
		t.Fatalf("bun state with bunx.exe = %#v, want running", bunState)
	}
}
