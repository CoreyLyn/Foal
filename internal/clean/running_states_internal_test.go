package clean

import (
	"reflect"
	"testing"
)

func TestMergeRunningApplicationStatesPreservesFirstSeenOrderAndReplacesInPlace(t *testing.T) {
	t.Parallel()

	merged := mergeRunningApplicationStates(nil,
		RunningApplicationState{Application: ApplicationGoogleChrome, State: RunningApplicationStateIdle},
		RunningApplicationState{Application: ApplicationMicrosoftEdge, State: RunningApplicationStateIdle},
		RunningApplicationState{Application: ApplicationVisualStudioCode, State: RunningApplicationStateIdle},
	)
	// Later observation for Chrome supersedes without reordering.
	merged = mergeRunningApplicationStates(merged,
		RunningApplicationState{Application: ApplicationGoogleChrome, State: RunningApplicationStateRunning},
		RunningApplicationState{Application: ApplicationCursor, State: RunningApplicationStateUnknown, Message: "snapshot failed"},
	)

	want := []RunningApplicationState{
		{Application: ApplicationGoogleChrome, State: RunningApplicationStateRunning},
		{Application: ApplicationMicrosoftEdge, State: RunningApplicationStateIdle},
		{Application: ApplicationVisualStudioCode, State: RunningApplicationStateIdle},
		{Application: ApplicationCursor, State: RunningApplicationStateUnknown, Message: "snapshot failed"},
	}
	if !reflect.DeepEqual(merged, want) {
		t.Fatalf("merged = %#v, want %#v", merged, want)
	}
}

func TestMergeRunningApplicationStatesIgnoresEmptyBatch(t *testing.T) {
	t.Parallel()

	existing := []RunningApplicationState{
		{Application: ApplicationCursor, State: RunningApplicationStateIdle},
	}
	got := mergeRunningApplicationStates(existing)
	if !reflect.DeepEqual(got, existing) {
		t.Fatalf("empty merge mutated slice: %#v", got)
	}
}

func TestProjectRunningApplicationStatesKeepsAllowedIdentitiesOnly(t *testing.T) {
	t.Parallel()

	states := []RunningApplicationState{
		{Application: ApplicationGoogleChrome, State: RunningApplicationStateRunning},
		{Application: ApplicationBun, State: RunningApplicationStateRunning},
		{Application: ApplicationMicrosoftEdge, State: RunningApplicationStateIdle},
		{Application: ApplicationNode, State: RunningApplicationStateRunning},
		{Application: ApplicationGoogleChrome, State: RunningApplicationStateUnknown, Message: "later"},
	}
	got := projectRunningApplicationStates(states,
		ApplicationGoogleChrome,
		ApplicationMicrosoftEdge,
	)
	want := []RunningApplicationState{
		{Application: ApplicationGoogleChrome, State: RunningApplicationStateUnknown, Message: "later"},
		{Application: ApplicationMicrosoftEdge, State: RunningApplicationStateIdle},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projected = %#v, want %#v", got, want)
	}
}
