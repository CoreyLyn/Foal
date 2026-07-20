package clean_test

import (
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// TestServicingOperationLines proves the path-free, byte-free human rendering of
// each servicing outcome: authorization skip, elevation denial/failure, no work,
// success, restart, failure, and cancellation.
func TestServicingOperationLines(t *testing.T) {
	exit0 := 0
	exit3010 := 3010
	exit3017 := 3017
	cases := []struct {
		name string
		op   clean.ServicingOperation
		want []string
	}{
		{
			name: "success",
			op:   clean.ServicingOperation{Outcome: clean.ServicingOutcomeCompleted, ReclaimablePackages: 4, CleanupRecommended: true, ExitCode: &exit0},
			want: []string{"completed", "4 reclaimable packages"},
		},
		{
			name: "success restart",
			op:   clean.ServicingOperation{Outcome: clean.ServicingOutcomeCompleted, ReclaimablePackages: 4, CleanupRecommended: true, ExitCode: &exit3010, RestartRequired: true},
			want: []string{"completed", "restart is required"},
		},
		{
			name: "failure restart",
			op:   clean.ServicingOperation{Outcome: clean.ServicingOutcomeFailed, Reason: clean.ServicingReasonCleanupFailed, ExitCode: &exit3017, RestartRequired: true},
			want: []string{"failed", "restart is required"},
		},
		{
			name: "no work",
			op:   clean.ServicingOperation{Outcome: clean.ServicingOutcomeNoWork},
			want: []string{"no cleanup needed"},
		},
		{
			name: "not authorized",
			op:   clean.ServicingOperation{Outcome: clean.ServicingOutcomeSkipped, Reason: clean.ServicingReasonNotAuthorized},
			want: []string{"skipped", "--allow-servicing"},
		},
		{
			name: "elevation denied",
			op:   clean.ServicingOperation{Outcome: clean.ServicingOutcomeSkipped, Reason: clean.ServicingReasonElevationDenied},
			want: []string{"skipped", "declined"},
		},
		{
			name: "elevation failed",
			op:   clean.ServicingOperation{Outcome: clean.ServicingOutcomeFailed, Reason: clean.ServicingReasonElevationFailed},
			want: []string{"failed"},
		},
		{
			name: "canceled",
			op:   clean.ServicingOperation{Outcome: clean.ServicingOutcomeCanceled, Reason: clean.ServicingReasonContextCanceled},
			want: []string{"canceled before servicing started"},
		},
		{
			name: "post-start cancel completed",
			op:   clean.ServicingOperation{Outcome: clean.ServicingOutcomeCompleted, ReclaimablePackages: 4, CleanupRecommended: true, ExitCode: &exit0, CancelRequested: true},
			want: []string{"completed", "Cancellation was requested"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := clean.ServicingOperationLines([]clean.ServicingOperation{tc.op})
			if len(lines) != 1 {
				t.Fatalf("lines = %#v, want exactly 1", lines)
			}
			line := lines[0]
			for _, want := range tc.want {
				if !strings.Contains(line, want) {
					t.Fatalf("line %q missing %q", line, want)
				}
			}
			// Human lines are path-free and byte-free: no WinSxS path, no raw DISM
			// output, no byte figure.
			for _, forbidden := range []string{`C:\`, "WinSxS", "dism", "stdout", "bytes"} {
				if strings.Contains(line, forbidden) {
					t.Fatalf("line %q leaked %q", line, forbidden)
				}
			}
		})
	}
}

// TestServicingOperationLinesEmpty proves no lines are emitted without operations.
func TestServicingOperationLinesEmpty(t *testing.T) {
	if lines := clean.ServicingOperationLines(nil); lines != nil {
		t.Fatalf("empty operations produced lines: %#v", lines)
	}
}
