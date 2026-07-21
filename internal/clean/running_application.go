package clean

import (
	"context"
	"strings"
)

type processSnapshot struct {
	Names []string
	Err   error
}

// serviceSnapshot is a read-only snapshot of Windows service states.
// NonStopped contains the service names that are not in SERVICE_STOPPED state.
type serviceSnapshot struct {
	NonStopped []string
	Err        error
}

func DetectSupportedBrowserApplications(ctx context.Context) []RunningApplicationState {
	return detectSupportedBrowserApplications(ctx, snapshotProcesses, snapshotServices)
}

func detectSupportedBrowserApplications(ctx context.Context, snapshotProcess func(context.Context) processSnapshot, snapshotService func(context.Context) serviceSnapshot) []RunningApplicationState {
	processResult := snapshotProcess(ctx)
	serviceResult := snapshotService(ctx)
	if processResult.Err != nil || serviceResult.Err != nil {
		var message string
		if processResult.Err != nil {
			message = processResult.Err.Error()
		} else {
			message = serviceResult.Err.Error()
		}
		return []RunningApplicationState{
			{Application: ApplicationGoogleChrome, State: RunningApplicationStateUnknown, Message: message},
			{Application: ApplicationMicrosoftEdge, State: RunningApplicationStateUnknown, Message: message},
			{Application: ApplicationMozillaFirefox, State: RunningApplicationStateUnknown, Message: message},
		}
	}
	return []RunningApplicationState{
		classifyBrowser(ApplicationGoogleChrome, "chrome.exe", nil, processResult.Names, serviceResult.NonStopped),
		classifyBrowser(ApplicationMicrosoftEdge, "msedge.exe", nil, processResult.Names, serviceResult.NonStopped),
		classifyBrowser(ApplicationMozillaFirefox, "firefox.exe", nil, processResult.Names, serviceResult.NonStopped),
	}
}

// classifyBrowser marks an application running when any of its registered
// executable names appears in the process snapshot OR any of its registered
// service names appears in the non-stopped service snapshot.
func classifyBrowser(application, executable string, services []string, processNames, serviceNames []string) RunningApplicationState {
	for _, name := range processNames {
		if strings.EqualFold(name, executable) {
			return RunningApplicationState{Application: application, State: RunningApplicationStateRunning}
		}
	}
	for _, name := range serviceNames {
		for _, service := range services {
			if strings.EqualFold(name, service) {
				return RunningApplicationState{Application: application, State: RunningApplicationStateRunning}
			}
		}
	}
	return RunningApplicationState{Application: application, State: RunningApplicationStateIdle}
}

// classifyBrowserProcess remains for backward compatibility in tests.
func classifyBrowserProcess(application, executable string, processNames []string) RunningApplicationState {
	return classifyBrowser(application, executable, nil, processNames, nil)
}

// DetectSupportedApplications detects both browsers and developer tools.
// Developer-tool process requirements come from the controlled application
// registry so one logical application may match multiple executable names.
func DetectSupportedApplications(ctx context.Context) []RunningApplicationState {
	return detectSupportedApplications(ctx, snapshotProcesses, snapshotServices)
}

func detectSupportedApplications(ctx context.Context, snapshotProcess func(context.Context) processSnapshot, snapshotService func(context.Context) serviceSnapshot) []RunningApplicationState {
	processResult := snapshotProcess(ctx)
	serviceResult := snapshotService(ctx)
	developerApps := developerApplicationDefinitions
	applicationCacheApps := applicationCacheApplicationDefinitions
	if processResult.Err != nil || serviceResult.Err != nil {
		var message string
		if processResult.Err != nil {
			message = processResult.Err.Error()
		} else {
			message = serviceResult.Err.Error()
		}
		states := []RunningApplicationState{
			{Application: ApplicationGoogleChrome, State: RunningApplicationStateUnknown, Message: message},
			{Application: ApplicationMicrosoftEdge, State: RunningApplicationStateUnknown, Message: message},
			{Application: ApplicationMozillaFirefox, State: RunningApplicationStateUnknown, Message: message},
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
		classifyApplication(ApplicationGoogleChrome, []string{"chrome.exe"}, nil, processResult.Names, serviceResult.NonStopped),
		classifyApplication(ApplicationMicrosoftEdge, []string{"msedge.exe"}, nil, processResult.Names, serviceResult.NonStopped),
		classifyApplication(ApplicationMozillaFirefox, []string{"firefox.exe"}, nil, processResult.Names, serviceResult.NonStopped),
	}
	for _, app := range developerApps {
		states = append(states, classifyApplication(app.id, app.executables, app.services, processResult.Names, serviceResult.NonStopped))
	}
	for _, app := range applicationCacheApps {
		states = append(states, classifyApplication(app.id, app.executables, app.services, processResult.Names, serviceResult.NonStopped))
	}
	return states
}

// classifyApplication marks an application running when any of its registered
// executable names appears in the process snapshot OR any of its registered
// service names appears in the non-stopped service snapshot.
func classifyApplication(application string, executables, services []string, processNames, serviceNames []string) RunningApplicationState {
	for _, name := range processNames {
		for _, executable := range executables {
			if strings.EqualFold(name, executable) {
				return RunningApplicationState{Application: application, State: RunningApplicationStateRunning}
			}
		}
	}
	for _, name := range serviceNames {
		for _, service := range services {
			if strings.EqualFold(name, service) {
				return RunningApplicationState{Application: application, State: RunningApplicationStateRunning}
			}
		}
	}
	return RunningApplicationState{Application: application, State: RunningApplicationStateIdle}
}

// classifyProcessByExecutables marks an application running when any of its
// registered executable names appears in the process snapshot.
func classifyProcessByExecutables(application string, executables []string, processNames []string) RunningApplicationState {
	return classifyApplication(application, executables, nil, processNames, nil)
}

// classifyProcess remains for single-executable call sites and tests.
func classifyProcess(application, executable string, processNames []string) RunningApplicationState {
	return classifyApplication(application, []string{executable}, nil, processNames, nil)
}
