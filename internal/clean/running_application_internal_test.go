package clean

import (
	"context"
	"errors"
	"testing"
)

func TestDetectSupportedBrowserApplicationsClassifiesRunningIdleAndUnknown(t *testing.T) {
	running := detectSupportedBrowserApplications(context.Background(), func(context.Context) processSnapshot {
		return processSnapshot{Names: []string{"notepad.exe", "chrome.exe", "msedge.exe"}}
	})
	if running[0].Application != ApplicationGoogleChrome || running[0].State != RunningApplicationStateRunning {
		t.Fatalf("Chrome state = %#v, want running", running[0])
	}
	if running[1].Application != ApplicationMicrosoftEdge || running[1].State != RunningApplicationStateRunning {
		t.Fatalf("Edge state = %#v, want running", running[1])
	}

	idle := detectSupportedBrowserApplications(context.Background(), func(context.Context) processSnapshot {
		return processSnapshot{Names: []string{"notepad.exe"}}
	})
	if idle[0].State != RunningApplicationStateIdle || idle[1].State != RunningApplicationStateIdle {
		t.Fatalf("idle states = %#v, want both idle", idle)
	}

	unknown := detectSupportedBrowserApplications(context.Background(), func(context.Context) processSnapshot {
		return processSnapshot{Err: errors.New("snapshot failed")}
	})
	if unknown[0].State != RunningApplicationStateUnknown || unknown[1].State != RunningApplicationStateUnknown {
		t.Fatalf("unknown states = %#v, want both unknown", unknown)
	}
	if unknown[0].Message == "" || unknown[1].Message == "" {
		t.Fatalf("unknown states = %#v, want diagnostic messages", unknown)
	}
}

func TestDetectSupportedApplicationsUsesRegisteredDeveloperTools(t *testing.T) {
	states := detectSupportedApplications(context.Background(), func(context.Context) processSnapshot {
		return processSnapshot{Names: []string{"go.exe", "cargo.exe", "notepad.exe"}}
	})
	// Browsers first, then developer applications in registry order.
	wantOrder := []string{
		ApplicationGoogleChrome,
		ApplicationMicrosoftEdge,
		ApplicationGo,
		ApplicationCargo,
		ApplicationDotNet,
		ApplicationNuGet,
		ApplicationNode,
		ApplicationPython,
	}
	if len(states) != len(wantOrder) {
		t.Fatalf("states = %#v, want %d entries", states, len(wantOrder))
	}
	for i, app := range wantOrder {
		if states[i].Application != app {
			t.Fatalf("states[%d].Application = %q, want %q", i, states[i].Application, app)
		}
	}
	if states[2].State != RunningApplicationStateRunning {
		t.Fatalf("go state = %#v, want running", states[2])
	}
	if states[3].State != RunningApplicationStateRunning {
		t.Fatalf("cargo state = %#v, want running", states[3])
	}
	if states[4].State != RunningApplicationStateIdle {
		t.Fatalf("dotnet state = %#v, want idle", states[4])
	}
}

func TestClassifyProcessByExecutablesSupportsMultipleNames(t *testing.T) {
	// One logical application with two executable names (future uv/uvx shape).
	executables := []string{"tool.exe", "toolx.exe"}
	if got := classifyProcessByExecutables("tool", executables, []string{"other.exe", "toolx.exe"}); got.State != RunningApplicationStateRunning {
		t.Fatalf("toolx.exe state = %#v, want running", got)
	}
	if got := classifyProcessByExecutables("tool", executables, []string{"TOOL.EXE"}); got.State != RunningApplicationStateRunning {
		t.Fatalf("case-insensitive tool.exe state = %#v, want running", got)
	}
	if got := classifyProcessByExecutables("tool", executables, []string{"notepad.exe"}); got.State != RunningApplicationStateIdle {
		t.Fatalf("idle state = %#v, want idle", got)
	}
}
