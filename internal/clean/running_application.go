package clean

import (
	"context"
	"strings"
)

type processSnapshot struct {
	Names []string
	Err   error
}

func DetectSupportedBrowserApplications(ctx context.Context) []RunningApplicationState {
	return detectSupportedBrowserApplications(ctx, snapshotProcesses)
}

func detectSupportedBrowserApplications(ctx context.Context, snapshot func(context.Context) processSnapshot) []RunningApplicationState {
	snapshotResult := snapshot(ctx)
	if snapshotResult.Err != nil {
		message := snapshotResult.Err.Error()
		return []RunningApplicationState{
			{Application: ApplicationGoogleChrome, State: RunningApplicationStateUnknown, Message: message},
			{Application: ApplicationMicrosoftEdge, State: RunningApplicationStateUnknown, Message: message},
		}
	}
	return []RunningApplicationState{
		classifyBrowserProcess(ApplicationGoogleChrome, "chrome.exe", snapshotResult.Names),
		classifyBrowserProcess(ApplicationMicrosoftEdge, "msedge.exe", snapshotResult.Names),
	}
}

func classifyBrowserProcess(application, executable string, processNames []string) RunningApplicationState {
	for _, name := range processNames {
		if strings.EqualFold(name, executable) {
			return RunningApplicationState{Application: application, State: RunningApplicationStateRunning}
		}
	}
	return RunningApplicationState{Application: application, State: RunningApplicationStateIdle}
}
