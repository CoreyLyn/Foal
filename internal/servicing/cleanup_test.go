package servicing

import (
	"net"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
)

func TestClassifyCleanupExit(t *testing.T) {
	cases := []struct {
		exit        int
		wantOutcome clean.ServicingOutcome
		wantReason  string
		wantRestart bool
	}{
		{0, clean.ServicingOutcomeCompleted, "", false},
		{3010, clean.ServicingOutcomeCompleted, "", true},
		{3017, clean.ServicingOutcomeFailed, clean.ServicingReasonCleanupFailed, true},
		{1, clean.ServicingOutcomeFailed, clean.ServicingReasonCleanupFailed, false},
		{2, clean.ServicingOutcomeFailed, clean.ServicingReasonCleanupFailed, false},
		{5, clean.ServicingOutcomeFailed, clean.ServicingReasonCleanupFailed, false},
	}
	for _, tc := range cases {
		outcome, reason, restart := classifyCleanupExit(tc.exit)
		if outcome != tc.wantOutcome || reason != tc.wantReason || restart != tc.wantRestart {
			t.Fatalf("classifyCleanupExit(%d) = (%q,%q,%v), want (%q,%q,%v)",
				tc.exit, outcome, reason, restart, tc.wantOutcome, tc.wantReason, tc.wantRestart)
		}
	}
}

func TestProjectAnalysisForExecute(t *testing.T) {
	exit0 := 0
	exit1 := 1

	t.Run("ready proceeds", func(t *testing.T) {
		proceed, _ := projectAnalysisForExecute(clean.ServicingAnalysisResult{
			Outcome: clean.ServicingOutcomeReady, ReclaimablePackages: 4, CleanupRecommended: true, ExitCode: &exit0,
		})
		if !proceed {
			t.Fatal("ready analysis must proceed to cleanup")
		}
	})
	t.Run("no_work does not proceed", func(t *testing.T) {
		proceed, res := projectAnalysisForExecute(clean.ServicingAnalysisResult{
			Outcome: clean.ServicingOutcomeNoWork, ReclaimablePackages: 0, CleanupRecommended: false, ExitCode: &exit0,
		})
		if proceed || res.Outcome != clean.ServicingOutcomeNoWork || res.ExitCode == nil || *res.ExitCode != 0 {
			t.Fatalf("no_work projection = proceed=%v res=%#v", proceed, res)
		}
	})
	t.Run("analysis failed does not proceed", func(t *testing.T) {
		proceed, res := projectAnalysisForExecute(clean.ServicingAnalysisResult{
			Outcome: clean.ServicingOutcomeFailed, Reason: clean.ServicingReasonAnalysisFailed, ExitCode: &exit1,
		})
		if proceed || res.Outcome != clean.ServicingOutcomeFailed || res.Reason != clean.ServicingReasonAnalysisFailed {
			t.Fatalf("failed projection = proceed=%v res=%#v", proceed, res)
		}
	})
	t.Run("invalid output does not proceed", func(t *testing.T) {
		proceed, res := projectAnalysisForExecute(clean.ServicingAnalysisResult{
			Outcome: clean.ServicingOutcomeFailed, Reason: clean.ServicingReasonAnalysisOutputInvalid, ExitCode: &exit0,
		})
		if proceed || res.Reason != clean.ServicingReasonAnalysisOutputInvalid {
			t.Fatalf("invalid-output projection = proceed=%v res=%#v", proceed, res)
		}
	})
	t.Run("skipped does not proceed", func(t *testing.T) {
		proceed, res := projectAnalysisForExecute(clean.ServicingAnalysisResult{
			Outcome: clean.ServicingOutcomeSkipped, Reason: clean.ServicingReasonUnsupportedPlatform,
		})
		if proceed || res.Outcome != clean.ServicingOutcomeSkipped {
			t.Fatalf("skipped projection = proceed=%v res=%#v", proceed, res)
		}
	})
}

func TestCleanupResultFromExit(t *testing.T) {
	analysis := clean.ServicingAnalysisResult{Outcome: clean.ServicingOutcomeReady, ReclaimablePackages: 4, CleanupRecommended: true}
	observed := func(v int64) *int64 { return &v }
	t.Run("completed", func(t *testing.T) {
		res := cleanupResultFromExit(analysis, 0, nil)
		if res.Outcome != clean.ServicingOutcomeCompleted || res.RestartRequired || res.ReclaimablePackages != 4 || !res.CleanupRecommended {
			t.Fatalf("completed = %#v", res)
		}
		if res.ExitCode == nil || *res.ExitCode != 0 {
			t.Fatalf("completed exit = %#v", res.ExitCode)
		}
	})
	t.Run("completed attaches positive observation", func(t *testing.T) {
		res := cleanupResultFromExit(analysis, 0, observed(1500))
		if res.ObservedFreeBytes == nil || *res.ObservedFreeBytes != 1500 {
			t.Fatalf("observation = %#v", res.ObservedFreeBytes)
		}
	})
	t.Run("completed records measured zero", func(t *testing.T) {
		res := cleanupResultFromExit(analysis, 0, observed(0))
		if res.ObservedFreeBytes == nil || *res.ObservedFreeBytes != 0 {
			t.Fatalf("measured zero must be recorded distinct from nil: %#v", res.ObservedFreeBytes)
		}
	})
	t.Run("completed restart drops observation", func(t *testing.T) {
		res := cleanupResultFromExit(analysis, 3010, observed(1500))
		if res.Outcome != clean.ServicingOutcomeCompleted || !res.RestartRequired {
			t.Fatalf("restart = %#v", res)
		}
		if res.ObservedFreeBytes != nil {
			t.Fatalf("restart-required must not attach observation: %#v", res.ObservedFreeBytes)
		}
	})
	t.Run("failed restart drops observation", func(t *testing.T) {
		res := cleanupResultFromExit(analysis, 3017, observed(1500))
		if res.Outcome != clean.ServicingOutcomeFailed || res.Reason != clean.ServicingReasonCleanupFailed || !res.RestartRequired {
			t.Fatalf("failed restart = %#v", res)
		}
		if res.ObservedFreeBytes != nil {
			t.Fatalf("failure must not attach observation: %#v", res.ObservedFreeBytes)
		}
	})
	t.Run("failed no restart drops observation", func(t *testing.T) {
		res := cleanupResultFromExit(analysis, 1, observed(1500))
		if res.Outcome != clean.ServicingOutcomeFailed || res.RestartRequired {
			t.Fatalf("failed no restart = %#v", res)
		}
		if res.ObservedFreeBytes != nil {
			t.Fatalf("failure must not attach observation: %#v", res.ObservedFreeBytes)
		}
	})
}

// TestExecuteExchangeRoundTrip proves the composite execute capability is
// transmitted over the one-request protocol and its restart-required and
// cancellation-request state round-trips back to the coordinator.
func TestExecuteExchangeRoundTrip(t *testing.T) {
	serverConn, helperConn := net.Pipe()
	defer serverConn.Close()
	defer helperConn.Close()

	exit3010 := 3010
	var gotCapability wireCapability
	helperErr := make(chan error, 1)
	go func() {
		helperErr <- helperExchange(helperConn, "n", func(capability wireCapability) pipeResponse {
			gotCapability = capability
			return responseFromExecute(clean.ServicingExecuteResult{
				Outcome:             clean.ServicingOutcomeCompleted,
				ReclaimablePackages: 4,
				CleanupRecommended:  true,
				ExitCode:            &exit3010,
				RestartRequired:     true,
			})
		})
	}()

	_ = serverConn.SetDeadline(time.Now().Add(5 * time.Second))
	resp, err := serverExchange(serverConn, "n", wireCapabilityExecuteComponentStoreCleanup)
	if err != nil {
		t.Fatalf("server exchange: %v", err)
	}
	if hErr := <-helperErr; hErr != nil {
		t.Fatalf("helper exchange: %v", hErr)
	}
	if gotCapability != wireCapabilityExecuteComponentStoreCleanup {
		t.Fatalf("helper received capability %d, want execute", gotCapability)
	}
	res := executeResultFromResponse(resp)
	if res.Outcome != clean.ServicingOutcomeCompleted || !res.RestartRequired ||
		res.ReclaimablePackages != 4 || !res.CleanupRecommended {
		t.Fatalf("execute round-trip lost fields: %#v", res)
	}
	if res.ExitCode == nil || *res.ExitCode != 3010 {
		t.Fatalf("execute round-trip exit = %#v", res.ExitCode)
	}
}

// TestExecuteExchangeObservationRoundTrip proves the optional free-space
// observation survives the wire, preserving the distinction between a measured
// value (including zero) and "not measured" (nil).
func TestExecuteExchangeObservationRoundTrip(t *testing.T) {
	observed := func(v int64) *int64 { return &v }
	cases := []struct {
		name string
		in   *int64
	}{
		{"positive", observed(2048)},
		{"measured zero", observed(0)},
		{"not measured", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			serverConn, helperConn := net.Pipe()
			defer serverConn.Close()
			defer helperConn.Close()

			exit0 := 0
			helperErr := make(chan error, 1)
			go func() {
				helperErr <- helperExchange(helperConn, "n", func(wireCapability) pipeResponse {
					return responseFromExecute(clean.ServicingExecuteResult{
						Outcome:           clean.ServicingOutcomeCompleted,
						ExitCode:          &exit0,
						ObservedFreeBytes: tc.in,
					})
				})
			}()

			_ = serverConn.SetDeadline(time.Now().Add(5 * time.Second))
			resp, err := serverExchange(serverConn, "n", wireCapabilityExecuteComponentStoreCleanup)
			if err != nil {
				t.Fatalf("server exchange: %v", err)
			}
			if hErr := <-helperErr; hErr != nil {
				t.Fatalf("helper exchange: %v", hErr)
			}
			res := executeResultFromResponse(resp)
			switch {
			case tc.in == nil && res.ObservedFreeBytes != nil:
				t.Fatalf("not-measured must round-trip nil: %#v", res.ObservedFreeBytes)
			case tc.in != nil && (res.ObservedFreeBytes == nil || *res.ObservedFreeBytes != *tc.in):
				t.Fatalf("observation lost: got %#v want %d", res.ObservedFreeBytes, *tc.in)
			}
		})
	}
}
