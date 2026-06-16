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
