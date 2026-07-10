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

// DetectSupportedApplications detects both browsers and developer tools
func DetectSupportedApplications(ctx context.Context) []RunningApplicationState {
	return detectSupportedApplications(ctx, snapshotProcesses)
}

func detectSupportedApplications(ctx context.Context, snapshot func(context.Context) processSnapshot) []RunningApplicationState {
	snapshotResult := snapshot(ctx)
	if snapshotResult.Err != nil {
		message := snapshotResult.Err.Error()
		return []RunningApplicationState{
			{Application: ApplicationGoogleChrome, State: RunningApplicationStateUnknown, Message: message},
			{Application: ApplicationMicrosoftEdge, State: RunningApplicationStateUnknown, Message: message},
			{Application: ApplicationGo, State: RunningApplicationStateUnknown, Message: message},
			{Application: ApplicationCargo, State: RunningApplicationStateUnknown, Message: message},
			{Application: ApplicationDotNet, State: RunningApplicationStateUnknown, Message: message},
			{Application: ApplicationNuGet, State: RunningApplicationStateUnknown, Message: message},
			{Application: ApplicationNode, State: RunningApplicationStateUnknown, Message: message},
			{Application: ApplicationPython, State: RunningApplicationStateUnknown, Message: message},
		}
	}
	return []RunningApplicationState{
		classifyProcess(ApplicationGoogleChrome, "chrome.exe", snapshotResult.Names),
		classifyProcess(ApplicationMicrosoftEdge, "msedge.exe", snapshotResult.Names),
		classifyProcess(ApplicationGo, "go.exe", snapshotResult.Names),
		classifyProcess(ApplicationCargo, "cargo.exe", snapshotResult.Names),
		classifyProcess(ApplicationDotNet, "dotnet.exe", snapshotResult.Names),
		classifyProcess(ApplicationNuGet, "nuget.exe", snapshotResult.Names),
		classifyProcess(ApplicationNode, "node.exe", snapshotResult.Names),
		classifyProcess(ApplicationPython, "python.exe", snapshotResult.Names),
	}
}

func classifyProcess(application, executable string, processNames []string) RunningApplicationState {
	for _, name := range processNames {
		if strings.EqualFold(name, executable) {
			return RunningApplicationState{Application: application, State: RunningApplicationStateRunning}
		}
	}
	return RunningApplicationState{Application: application, State: RunningApplicationStateIdle}
}

