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

// DetectSupportedApplications detects both browsers and developer tools.
// Developer-tool process requirements come from the controlled application
// registry so one logical application may match multiple executable names.
func DetectSupportedApplications(ctx context.Context) []RunningApplicationState {
	return detectSupportedApplications(ctx, snapshotProcesses)
}

func detectSupportedApplications(ctx context.Context, snapshot func(context.Context) processSnapshot) []RunningApplicationState {
	snapshotResult := snapshot(ctx)
	developerApps := developerApplicationDefinitions
	applicationCacheApps := applicationCacheApplicationDefinitions
	if snapshotResult.Err != nil {
		message := snapshotResult.Err.Error()
		states := []RunningApplicationState{
			{Application: ApplicationGoogleChrome, State: RunningApplicationStateUnknown, Message: message},
			{Application: ApplicationMicrosoftEdge, State: RunningApplicationStateUnknown, Message: message},
		}
		for _, app := range developerApps {
			states = append(states, RunningApplicationState{
				Application: app.id,
				State:       RunningApplicationStateUnknown,
				Message:     message,
			})
		}
		for _, app := range applicationCacheApps {
			states = append(states, RunningApplicationState{
				Application: app.id,
				State:       RunningApplicationStateUnknown,
				Message:     message,
			})
		}
		return states
	}
	states := []RunningApplicationState{
		classifyBrowserProcess(ApplicationGoogleChrome, "chrome.exe", snapshotResult.Names),
		classifyBrowserProcess(ApplicationMicrosoftEdge, "msedge.exe", snapshotResult.Names),
	}
	for _, app := range developerApps {
		states = append(states, classifyProcessByExecutables(app.id, app.executables, snapshotResult.Names))
	}
	for _, app := range applicationCacheApps {
		states = append(states, classifyProcessByExecutables(app.id, app.executables, snapshotResult.Names))
	}
	return states
}

// classifyProcessByExecutables marks an application running when any of its
// registered executable names appears in the process snapshot.
func classifyProcessByExecutables(application string, executables []string, processNames []string) RunningApplicationState {
	for _, name := range processNames {
		for _, executable := range executables {
			if strings.EqualFold(name, executable) {
				return RunningApplicationState{Application: application, State: RunningApplicationStateRunning}
			}
		}
	}
	return RunningApplicationState{Application: application, State: RunningApplicationStateIdle}
}

// classifyProcess remains for single-executable call sites and tests.
func classifyProcess(application, executable string, processNames []string) RunningApplicationState {
	return classifyProcessByExecutables(application, []string{executable}, processNames)
}
