package uninstall

import "runtime"

type Evidence struct {
	Applications []ApplicationEvidence
	Leftovers    []LeftoverEvidence
}

type ApplicationEvidence struct {
	Name      string
	Version   string
	Publisher string
	Sources   []string
}

type LeftoverEvidence struct {
	Path    string
	App     string
	Signals []string
}

type Result struct {
	Status              string                  `json:"status"`
	Applications        []Application           `json:"applications"`
	EvidenceSources     []EvidenceSource        `json:"evidence_sources"`
	PossibleLeftovers   []LeftoverCandidate     `json:"possible_leftovers"`
	SharedStateConcerns []SharedStateConcern    `json:"shared_state_concerns"`
	UnknownState        []UnknownStateCandidate `json:"unknown_state"`
	Skipped             []SkippedReason         `json:"skipped"`
	Execution           ExecutionPolicy         `json:"execution"`
}

type Application struct {
	Name          string   `json:"name"`
	Version       string   `json:"version,omitempty"`
	Publisher     string   `json:"publisher,omitempty"`
	Evidence      []string `json:"evidence"`
	Confidence    string   `json:"confidence"`
	Ownership     string   `json:"ownership"`
	SkippedReason string   `json:"skipped_reason,omitempty"`
}

type EvidenceSource struct {
	Source string `json:"source"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type LeftoverCandidate struct {
	Path       string `json:"path"`
	App        string `json:"app,omitempty"`
	Ownership  string `json:"ownership"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
}

type SharedStateConcern struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type UnknownStateCandidate struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type SkippedReason struct {
	Source      string `json:"source"`
	Reason      string `json:"reason"`
	Recoverable bool   `json:"recoverable"`
}

type ExecutionPolicy struct {
	Allowed bool     `json:"allowed"`
	Actions []string `json:"actions"`
	Reason  string   `json:"reason"`
}

func Review() Result {
	sources := []EvidenceSource{
		{Source: "windows_registry_uninstall_keys", Status: "skipped", Reason: "discovery provider not implemented"},
		{Source: "known_leftover_locations", Status: "skipped", Reason: "discovery provider not implemented"},
	}
	skipped := []SkippedReason{
		{Source: "windows_registry_uninstall_keys", Reason: "discovery_provider_not_implemented", Recoverable: true},
		{Source: "known_leftover_locations", Reason: "discovery_provider_not_implemented", Recoverable: true},
	}
	if runtime.GOOS != "windows" {
		sources = append(sources, EvidenceSource{Source: "windows_only_evidence", Status: "skipped", Reason: "not running on Windows"})
		skipped = append(skipped, SkippedReason{Source: "windows_only_evidence", Reason: "unsupported_platform", Recoverable: true})
	}

	return Result{
		Status:              "preview",
		Applications:        []Application{},
		EvidenceSources:     sources,
		PossibleLeftovers:   []LeftoverCandidate{},
		SharedStateConcerns: []SharedStateConcern{},
		UnknownState:        []UnknownStateCandidate{},
		Skipped:             skipped,
		Execution:           previewOnlyExecution(),
	}
}

func ReviewEvidence(evidence Evidence) Result {
	result := Result{
		Status:              "preview",
		Applications:        []Application{},
		EvidenceSources:     evidenceSources(evidence),
		PossibleLeftovers:   []LeftoverCandidate{},
		SharedStateConcerns: []SharedStateConcern{},
		UnknownState:        []UnknownStateCandidate{},
		Skipped:             []SkippedReason{},
		Execution:           previewOnlyExecution(),
	}

	for _, appEvidence := range evidence.Applications {
		confidence := applicationConfidence(appEvidence)
		result.Applications = append(result.Applications, Application{
			Name:       appEvidence.Name,
			Version:    appEvidence.Version,
			Publisher:  appEvidence.Publisher,
			Evidence:   append([]string{}, appEvidence.Sources...),
			Confidence: confidence,
			Ownership:  ownershipForConfidence(confidence),
		})
	}

	for _, leftover := range evidence.Leftovers {
		switch classifyLeftover(leftover) {
		case "app_owned":
			result.PossibleLeftovers = append(result.PossibleLeftovers, LeftoverCandidate{
				Path:       leftover.Path,
				App:        leftover.App,
				Ownership:  "app_owned",
				Confidence: "high",
				Reason:     "leftover signals tie this path to one application",
			})
		case "shared_state":
			result.SharedStateConcerns = append(result.SharedStateConcerns, SharedStateConcern{
				Path:   leftover.Path,
				Reason: "candidate appears to contain shared application or publisher state",
			})
		default:
			result.UnknownState = append(result.UnknownState, UnknownStateCandidate{
				Path:   leftover.Path,
				Reason: "evidence is too weak for an ownership decision",
			})
		}
	}

	return result
}

func evidenceSources(evidence Evidence) []EvidenceSource {
	seen := map[string]bool{}
	var sources []EvidenceSource
	for _, app := range evidence.Applications {
		for _, source := range app.Sources {
			if !seen[source] {
				sources = append(sources, EvidenceSource{Source: source, Status: "reported"})
				seen[source] = true
			}
		}
	}
	if len(sources) == 0 {
		return []EvidenceSource{}
	}
	return sources
}

func applicationConfidence(evidence ApplicationEvidence) string {
	sourceCount := 0
	for _, source := range evidence.Sources {
		if source != "" {
			sourceCount++
		}
	}
	if evidence.Name != "" && sourceCount >= 2 {
		return "high"
	}
	if evidence.Name != "" && sourceCount == 1 {
		return "medium"
	}
	return "low"
}

func ownershipForConfidence(confidence string) string {
	if confidence == "high" {
		return "app_owned"
	}
	return "unknown"
}

func classifyLeftover(leftover LeftoverEvidence) string {
	if hasSignal(leftover.Signals, "shared_program_data") {
		return "shared_state"
	}
	if leftover.App != "" && hasSignal(leftover.Signals, "app_name_match") && hasSignal(leftover.Signals, "under_user_profile") {
		return "app_owned"
	}
	return "unknown"
}

func hasSignal(signals []string, want string) bool {
	for _, signal := range signals {
		if signal == want {
			return true
		}
	}
	return false
}

func previewOnlyExecution() ExecutionPolicy {
	return ExecutionPolicy{
		Allowed: false,
		Actions: []string{},
		Reason:  "uninstall is preview-only; Foal does not execute uninstallers, stop processes, or delete leftovers",
	}
}
