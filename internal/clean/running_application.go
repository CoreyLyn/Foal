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

// devCacheCategoryRequiresRunningCheck returns true if the dev cache category
// needs running-application checks (distinctive process tools)
func devCacheCategoryRequiresRunningCheck(category string) bool {
	switch category {
	case DevCacheCategoryGo, DevCacheCategoryCargo, DevCacheCategoryNuGet:
		return true
	default:
		return false
	}
}

// devCacheCategoryToApplications maps a dev cache category to the application(s)
// that indicate it's in use
func devCacheCategoryToApplications(category string) []string {
	switch category {
	case DevCacheCategoryGo:
		return []string{ApplicationGo}
	case DevCacheCategoryCargo:
		return []string{ApplicationCargo}
	case DevCacheCategoryNuGet:
		// Both dotnet.exe and nuget.exe can use the nuget cache
		return []string{ApplicationDotNet, ApplicationNuGet}
	default:
		// Runtime-hosted tools (npm, pip, corepack) don't need checks
		return nil
	}
}

// devToolIsRunningOrUnknown returns true if any of the given applications are
// running OR if any have unknown state (fail-closed)
func devToolIsRunningOrUnknown(states []RunningApplicationState, applications []string) bool {
	for _, app := range applications {
		state, ok := runningApplicationStateFor(states, app)
		if !ok || state.State == RunningApplicationStateRunning || state.State == RunningApplicationStateUnknown {
			return true
		}
	}
	return false
}
