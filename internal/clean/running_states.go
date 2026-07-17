package clean

// running_states.go is part of the running-gate module (see running_gate.go).
// These pure helpers merge and project RunningApplicationState slices by
// canonical application identity. Gate outcomes already apply them so category
// resolvers rarely call project helpers directly; multi-category merges and
// Result aggregation still use mergeRunningApplicationStates.

// mergeRunningApplicationStates projects observed running-application states
// into existing by canonical application identity.
//
// Rules:
//   - match on Application identity only
//   - preserve first-seen ordering for stable JSON/human output
//   - when the same identity is observed again, replace the entry in place
//     (post-measurement supersedes pre-measurement without reordering)
//
// Callers must already scope observed states to applications that participated
// in the current Clean plan; this helper does not invent relevance.
func mergeRunningApplicationStates(existing []RunningApplicationState, observed ...RunningApplicationState) []RunningApplicationState {
	if len(observed) == 0 {
		return existing
	}
	for _, state := range observed {
		replaced := false
		for index, current := range existing {
			if current.Application == state.Application {
				existing[index] = state
				replaced = true
				break
			}
		}
		if !replaced {
			existing = append(existing, state)
		}
	}
	return existing
}

// projectRunningApplicationStates keeps only states whose Application is listed
// in identities. Later observations of the same identity replace earlier ones
// in place; unrelated identities are dropped.
func projectRunningApplicationStates(states []RunningApplicationState, identities ...string) []RunningApplicationState {
	if len(states) == 0 || len(identities) == 0 {
		return nil
	}
	allowed := make(map[string]bool, len(identities))
	for _, identity := range identities {
		if identity == "" {
			continue
		}
		allowed[identity] = true
	}
	if len(allowed) == 0 {
		return nil
	}
	var projected []RunningApplicationState
	for _, state := range states {
		if !allowed[state.Application] {
			continue
		}
		projected = mergeRunningApplicationStates(projected, state)
	}
	return projected
}

// browserRunningApplicationIdentities returns the supported browser identities
// gated by browser-cache resolution (Chrome, Edge, and Firefox).
func browserRunningApplicationIdentities() []string {
	identities := make([]string, 0, len(browserCacheConfigs))
	for _, config := range browserCacheConfigs {
		identities = append(identities, config.application)
	}
	return identities
}
