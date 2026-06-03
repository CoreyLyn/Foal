package uninstall

import "testing"

func TestReviewEvidenceClassifiesApplicationsAndLeftovers(t *testing.T) {
	result := ReviewEvidence(Evidence{
		Applications: []ApplicationEvidence{
			{
				Name:      "Example App",
				Version:   "1.2.3",
				Publisher: "Example Co",
				Sources:   []string{"windows_registry_uninstall_keys", "install_location"},
			},
		},
		Leftovers: []LeftoverEvidence{
			{Path: `C:\Users\corey\AppData\Local\Example App\cache`, App: "Example App", Signals: []string{"app_name_match", "under_user_profile"}},
			{Path: `C:\ProgramData\Example Co\Shared`, App: "Example App", Signals: []string{"shared_program_data"}},
			{Path: `C:\Users\corey\AppData\Roaming\mystery`, Signals: []string{"weak_name_match"}},
		},
	})

	if len(result.Applications) != 1 {
		t.Fatalf("applications = %#v, want one", result.Applications)
	}
	app := result.Applications[0]
	if app.Confidence != "high" {
		t.Fatalf("app confidence = %q, want high", app.Confidence)
	}
	if app.Ownership != "app_owned" {
		t.Fatalf("app ownership = %q, want app_owned", app.Ownership)
	}
	if len(result.PossibleLeftovers) != 1 {
		t.Fatalf("possible leftovers = %#v, want one app-owned candidate", result.PossibleLeftovers)
	}
	if result.PossibleLeftovers[0].Ownership != "app_owned" || result.PossibleLeftovers[0].Confidence != "high" {
		t.Fatalf("leftover classification = %#v, want app_owned/high", result.PossibleLeftovers[0])
	}
	if len(result.SharedStateConcerns) != 1 {
		t.Fatalf("shared state concerns = %#v, want one", result.SharedStateConcerns)
	}
	if len(result.UnknownState) != 1 {
		t.Fatalf("unknown state = %#v, want one", result.UnknownState)
	}
	if result.Execution.Allowed {
		t.Fatal("execution allowed = true, want false")
	}
	if len(result.Execution.Actions) != 0 {
		t.Fatalf("execution actions = %#v, want empty", result.Execution.Actions)
	}
}

func TestReviewReportsSkippedDiscoveryProviders(t *testing.T) {
	result := Review()

	if len(result.Skipped) < 2 {
		t.Fatalf("skipped = %#v, want registry and leftover discovery skips", result.Skipped)
	}
	for _, skipped := range result.Skipped[:2] {
		if skipped.Reason != "discovery_provider_not_implemented" {
			t.Fatalf("skipped reason = %q, want discovery_provider_not_implemented", skipped.Reason)
		}
		if !skipped.Recoverable {
			t.Fatalf("skipped recoverable = false for %#v, want true", skipped)
		}
	}
	if result.Execution.Allowed {
		t.Fatal("execution allowed = true, want false")
	}
}
