package uninstall

import "runtime"

type Evidence struct {
	Applications    []ApplicationEvidence
	Leftovers       []LeftoverEvidence
	OrphanedResidue []OrphanedResidueEvidence
}

type ApplicationEvidence struct {
	Name                        string
	Version                     string
	Publisher                   string
	QuietUninstallCommand       string
	InteractiveUninstallCommand string
	InstallLocation             string
	Sources                     []string
}

type LeftoverEvidence struct {
	Path    string
	App     string
	Signals []string
}

type OrphanedResidueEvidence struct {
	Path       string
	SourceRoot string
}

type Result struct {
	Status              string                     `json:"status"`
	Applications        []Application              `json:"applications"`
	EvidenceSources     []EvidenceSource           `json:"evidence_sources"`
	ReviewSections      []ReviewSection            `json:"review_sections"`
	PossibleLeftovers   []LeftoverCandidate        `json:"possible_leftovers"`
	SharedStateConcerns []SharedStateConcern       `json:"shared_state_concerns"`
	OrphanedResidue     []OrphanedResidueCandidate `json:"orphaned_residue"`
	UnknownState        []UnknownStateCandidate    `json:"unknown_state"`
	Skipped             []SkippedReason            `json:"skipped"`
	Execution           ExecutionPolicy            `json:"execution"`
}

type Application struct {
	Name                        string   `json:"name"`
	Version                     string   `json:"version,omitempty"`
	Publisher                   string   `json:"publisher,omitempty"`
	QuietUninstallCommand       string   `json:"quiet_uninstall_command,omitempty"`
	InteractiveUninstallCommand string   `json:"interactive_uninstall_command,omitempty"`
	InstallLocation             string   `json:"install_location,omitempty"`
	PlannedClass                string   `json:"planned_class"`
	PlannedReason               string   `json:"planned_reason,omitempty"`
	Evidence                    []string `json:"evidence"`
	Confidence                  string   `json:"confidence"`
	Ownership                   string   `json:"ownership"`
	SkippedReason               string   `json:"skipped_reason,omitempty"`
}

type EvidenceSource struct {
	Source string `json:"source"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type ReviewSection struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Count int    `json:"count"`
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

type OrphanedResidueCandidate struct {
	Path       string `json:"path"`
	SourceRoot string `json:"source_root"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
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

type DiscoveryResult struct {
	Evidence Evidence
	Sources  []EvidenceSource
	Skipped  []SkippedReason
}

var discoverUninstallEvidence = discoverPlatformUninstallEvidence
var runtimeGOOS = runtime.GOOS

func Review() Result {
	discovery := discoverUninstallEvidence()
	leftover := discoverLeftoverEvidence(discovery.Evidence.Applications)
	orphaned := discoverOrphanedResidueEvidence(discovery.Evidence.Applications)
	discovery.Evidence.Leftovers = append(discovery.Evidence.Leftovers, leftover.Leftovers...)
	discovery.Evidence.OrphanedResidue = append(discovery.Evidence.OrphanedResidue, orphaned.Candidates...)

	result := ReviewEvidence(discovery.Evidence)

	result.EvidenceSources = append([]EvidenceSource{}, discovery.Sources...)
	result.Skipped = append([]SkippedReason{}, discovery.Skipped...)
	if len(result.EvidenceSources) == 0 && len(discovery.Evidence.Applications) > 0 {
		result.EvidenceSources = evidenceSources(discovery.Evidence)
	}

	result.EvidenceSources = append(result.EvidenceSources, leftover.Source)
	result.EvidenceSources = append(result.EvidenceSources, orphaned.Source)
	result.Skipped = append(result.Skipped, leftover.Skipped...)
	result.Skipped = append(result.Skipped, orphaned.Skipped...)

	if runtimeGOOS != "windows" && !hasSkippedSource(result.Skipped, "windows_only_evidence") {
		result.EvidenceSources = append(result.EvidenceSources, EvidenceSource{Source: "windows_only_evidence", Status: "skipped", Reason: "not running on Windows"})
		result.Skipped = append(result.Skipped, SkippedReason{Source: "windows_only_evidence", Reason: "unsupported_platform", Recoverable: true})
	}

	return WithReviewSections(result)
}

func hasSkippedSource(skipped []SkippedReason, source string) bool {
	for _, item := range skipped {
		if item.Source == source {
			return true
		}
	}
	return false
}

func ReviewEvidence(evidence Evidence) Result {
	result := Result{
		Status:              "preview",
		Applications:        []Application{},
		EvidenceSources:     evidenceSources(evidence),
		ReviewSections:      []ReviewSection{},
		PossibleLeftovers:   []LeftoverCandidate{},
		SharedStateConcerns: []SharedStateConcern{},
		OrphanedResidue:     []OrphanedResidueCandidate{},
		UnknownState:        []UnknownStateCandidate{},
		Skipped:             []SkippedReason{},
		Execution:           previewOnlyExecution(),
	}

	for _, appEvidence := range evidence.Applications {
		confidence := applicationConfidence(appEvidence)
		plannedClass, plannedReason := classifyApplicationPlan(appEvidence)
		app := Application{
			Name:          appEvidence.Name,
			Version:       appEvidence.Version,
			Publisher:     appEvidence.Publisher,
			PlannedClass:  plannedClass,
			PlannedReason: plannedReason,
			Evidence:      append([]string{}, appEvidence.Sources...),
			Confidence:    confidence,
			Ownership:     ownershipForConfidence(confidence),
		}
		// Surface uninstall command and install location evidence only for
		// apps Foal would consider executing. Hard exclusions remain listed
		// (so the user sees Foal recognized them) but their executable
		// evidence is suppressed so they are not presented as executable
		// targets.
		if plannedClass != PlannedClassHardExclusion {
			app.QuietUninstallCommand = appEvidence.QuietUninstallCommand
			app.InteractiveUninstallCommand = appEvidence.InteractiveUninstallCommand
			app.InstallLocation = appEvidence.InstallLocation
		}
		result.Applications = append(result.Applications, app)
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

	for _, orphaned := range evidence.OrphanedResidue {
		result.OrphanedResidue = append(result.OrphanedResidue, OrphanedResidueCandidate{
			Path:       orphaned.Path,
			SourceRoot: orphaned.SourceRoot,
			Confidence: "low",
			Reason:     "directory is under an application data root but does not match a discovered installed application or publisher",
		})
	}

	return WithReviewSections(result)
}

func WithReviewSections(result Result) Result {
	result.ReviewSections = []ReviewSection{
		{ID: "applications", Label: "Applications", Count: len(result.Applications)},
		{ID: "evidence_sources", Label: "Evidence sources", Count: len(result.EvidenceSources)},
		{ID: "possible_leftovers", Label: "Possible leftovers", Count: len(result.PossibleLeftovers)},
		{ID: "shared_state_concerns", Label: "Shared state concerns", Count: len(result.SharedStateConcerns)},
		{ID: "orphaned_residue", Label: "Orphaned residue", Count: len(result.OrphanedResidue)},
		{ID: "unknown_state", Label: "Unknown state", Count: len(result.UnknownState)},
		{ID: "skipped_discovery_sources", Label: "Skipped discovery sources", Count: len(result.Skipped)},
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
