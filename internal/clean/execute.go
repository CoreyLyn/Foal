package clean

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/delete"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// Execute runs shared Clean mutation for the effective category plan.
//
// Pipeline phases (ADR-0018 planned actions; ADR-0008 fresh selected-only resolve):
//  1. skeleton / plan
//  2. fresh resolve defaults + selected opt-ins (category core / opt-in resolver)
//  3. partition Recycle Bin vs permanent by catalog planned_action
//  4. aggregate Recycle Bin capacity preflight (RB candidates only)
//  5. mutate Recycle Bin first
//  6. mutate permanent last (authorization-gated; no RB fallback)
//  7. history record + completion progress
//
// Cancellation stops remaining work without rollback (ADR-0014). Permanent
// deletion is never a Recycle Bin fallback.
func Execute(ctx context.Context, opts Options) Result {
	return runExecute(ctx, opts)
}

func runExecute(ctx context.Context, opts Options) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	// Fresh resolve starts without a specific category; per-category Scanning
	// events follow at each resolve boundary (see appendDefaultCandidates /
	// resolveOptInCandidates).
	reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseScanning, "")

	// Phase 1: skeleton / plan
	if opts.ProtectionLoadError != nil {
		result := protectionLoadFailure("execute", opts, start)
		reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseComplete, "")
		return result
	}
	categoryPlan := effectiveCategoryPlan(opts)
	result := newCleanResultSkeleton("execute", opts)

	// Phase 2: fresh resolve defaults + selected opt-ins
	executionCandidates := resolveExecuteCandidates(ctx, opts, categoryPlan, &result)

	// Categories with no queued candidates are provisionally terminal after
	// resolve (empty / skipped / failed from evidence already on result).
	// Mutation phases never see them again.
	completedCategories := reportResolvedCategoryCompletions(opts, categoryPlan, executionCandidates, &result)

	// Phase 3: partition Recycle Bin vs permanent
	recycleBinCandidates, permanentCandidates := partitionByPlannedAction(executionCandidates)

	// Phase 4: aggregate Recycle Bin capacity preflight (RB only).
	// ActiveCategory is empty: this check is volume-scoped, not category-scoped.
	reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseRecycleBinSafety, "")
	executionGroups := prepareRecycleBinCandidateGroups(opts, recycleBinCandidates)

	// Phase 5: mutate Recycle Bin first (category ActiveCategory at each
	// first-touch boundary inside the volume-grouped path).
	adapter := opts.RecycleBinAdapter
	if adapter == nil {
		adapter = delete.WindowsRecycleBinAdapter{}
	}
	executeRecycleBinCandidateGroups(ctx, opts, adapter, executionGroups, &result, completedCategories)

	// Phase 6: mutate permanent last (per-category progress then mutation)
	if len(permanentCandidates) > 0 {
		executePermanentCandidates(ctx, opts, permanentCandidates, &result, completedCategories)
	}

	// Windows servicing is the final action group (ADR 0029). It runs only after
	// Recycle Bin and Permanent deletion work, and once a servicing operation
	// starts no later action begins. Missing --allow-servicing skips with
	// windows_servicing_not_authorized and never opens UAC; when authorized the
	// composite gateway performs a fresh analysis, enforces the guard, and starts
	// component cleanup within one authenticated helper session.
	appendServicingExecution(ctx, opts, planServicingCategories(categoryPlan), &result)

	// Phase 7: history + completion
	result.ElapsedMS = time.Since(start).Milliseconds()
	result.Totals = totals(result)
	recordHistorySession(ctx, opts, result, start, time.Now())
	reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseComplete, "")
	return result
}

// resolveExecuteCandidates fresh-resolves default and selected opt-in work into
// action-tagged candidates. No mutation occurs here.
func resolveExecuteCandidates(ctx context.Context, opts Options, categoryPlan CategoryPlan, result *Result) []actionExecutionCandidate {
	exactDefaults := categoryPlan.Mode == SelectionModeExact
	appendDefaultCandidates(ctx, opts, planDefaultSet(categoryPlan), exactDefaults, result)

	executionCandidates := make([]actionExecutionCandidate, 0, len(result.Candidates))
	for _, candidate := range result.Candidates {
		executionCandidates = append(executionCandidates, actionExecutionCandidate{
			candidate:     delete.Candidate{Path: candidate.Path, Bytes: candidate.Bytes},
			rule:          candidate.Rule,
			plannedAction: resolvePlannedAction(candidate.Rule, opts.CategoryPlannedActions),
		})
	}

	// Opt-in candidates: resolve fresh, then capacity-check and delete. The
	// resolver scans only planned opt-in categories (ADR-0008) and runs gating;
	// both modes share it, so execute surfaces browser running states and
	// diagnostics. Exact plans omit unlisted defaults and unselected opt-ins.
	optInPlan := planOptInSet(categoryPlan)
	if len(optInPlan) == 0 {
		return executionCandidates
	}
	resolution := resolveOptInCandidates(ctx, opts, optInPlan)
	result.RunningApplications = mergeRunningApplicationStates(result.RunningApplications, resolution.runningStates...)
	result.Errors = append(result.Errors, resolution.diagnostics...)
	result.Skipped = append(result.Skipped, resolution.skipped...)

	for _, c := range resolution.candidates {
		if opts.Validator.IsUserProtected(c.Path) {
			continue
		}
		// nvidia_installer_cache is a move_to_recycle_bin opt-in category. Its
		// candidate flows through the shared Recycle Bin phase like any other opt-in:
		// aggregate capacity preflight, then an action-neutral immediate category
		// identity revalidation (composeRecycleBinPreMutation) repeats the NVIDIA
		// proof right before the move. A capacity or revalidation failure is a
		// fail-closed skip and never falls back to permanent deletion.
		executionCandidates = append(executionCandidates, actionExecutionCandidate{
			candidate:     delete.Candidate{Path: c.Path, Bytes: c.Bytes},
			rule:          c.Category,
			isOptIn:       true,
			plannedAction: resolvePlannedAction(c.Category, opts.CategoryPlannedActions),
		})
	}
	return executionCandidates
}

// partitionByPlannedAction splits candidates into Recycle Bin vs permanent
// buckets using each candidate's planned action. Permanent bytes never enter
// Recycle Bin capacity preflight.
func partitionByPlannedAction(candidates []actionExecutionCandidate) (recycleBin, permanent []actionExecutionCandidate) {
	for _, candidate := range candidates {
		if candidate.plannedAction == string(PlannedActionDeletePermanently) {
			permanent = append(permanent, candidate)
			continue
		}
		recycleBin = append(recycleBin, candidate)
	}
	return recycleBin, permanent
}

// reportExecutionProgress delivers an observation-only event. activeCategory is
// the path-free canonical category id for the boundary being entered, or empty
// when the phase is not category-scoped. Panics in the reporter are recovered
// so observation can never interrupt or authorize execution.
func reportExecutionProgress(reporter ProgressReporter, phase ExecutionPhase, activeCategory string) {
	if reporter == nil {
		return
	}
	defer func() { _ = recover() }()
	reporter(ExecutionProgress{Phase: phase, ActiveCategory: activeCategory})
}

// reportCategoryCompletion emits a provisional mid-flight terminal for one
// category. completed tracks ids already reported so resolve and mutate do not
// double-complete. Panics in the reporter are recovered.
func reportCategoryCompletion(reporter ProgressReporter, phase ExecutionPhase, outcome CategoryExecutionOutcome, completed map[string]bool) {
	id := strings.TrimSpace(outcome.Identifier)
	if reporter == nil || id == "" || !IsTerminalExecutionState(outcome.State) {
		return
	}
	if completed != nil {
		if completed[id] {
			return
		}
		completed[id] = true
	}
	defer func() { _ = recover() }()
	reporter(ExecutionProgress{
		Phase:                            phase,
		CompletedCategory:                id,
		CompletedState:                   outcome.State,
		CompletedAffectedBytes:           outcome.AffectedBytes,
		CompletedRecycleBinMovedBytes:    outcome.RecycleBinMovedBytes,
		CompletedPermanentlyDeletedBytes: outcome.PermanentlyDeletedBytes,
		CompletedDeletedCount:            outcome.DeletedCount,
		CompletedSkippedCount:            outcome.SkippedCount,
	})
}

// reportResolvedCategoryCompletions marks selected categories with zero queued
// execution candidates as provisionally terminal after fresh resolve. Returns
// the completion set so mutate phases can record finished work without
// re-emitting. Path-free; never invents success for categories still pending.
func reportResolvedCategoryCompletions(opts Options, plan CategoryPlan, candidates []actionExecutionCandidate, result *Result) map[string]bool {
	completed := make(map[string]bool)
	if result == nil {
		return completed
	}
	hasCandidate := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if candidate.rule != "" {
			hasCandidate[candidate.rule] = true
		}
	}
	for _, id := range plan.Categories {
		if id == "" || hasCandidate[id] {
			continue
		}
		outcome := ProjectProvisionalCategoryOutcome(id, *result)
		reportCategoryCompletion(opts.ProgressReporter, ExecutionPhaseScanning, outcome, completed)
	}
	return completed
}

// actionExecutionCandidate is one freshly resolved candidate with its planned
// deletion action for the action-aware execution seam.
type actionExecutionCandidate struct {
	candidate     delete.Candidate
	rule          string
	isOptIn       bool
	plannedAction string
}

type recycleBinVolumeGroup struct {
	config     RecycleBinVolumeConfig
	candidates []actionExecutionCandidate
	totalBytes int64
	unsafe     bool
}

type recycleBinVolumeIdentity struct {
	key   string
	known bool
}

type recycleBinCandidateGroups struct {
	byVolume map[string]*recycleBinVolumeGroup
	order    []string
}

func prepareRecycleBinCandidateGroups(opts Options, candidates []actionExecutionCandidate) recycleBinCandidateGroups {
	probe := opts.RecycleBinCapacityProbe
	if probe == nil {
		probe = RecycleBinVolumeCapacity
	}

	groups := make(map[string]*recycleBinVolumeGroup)
	volumeOrder := make([]string, 0)
	for _, candidate := range candidates {
		cfg, err := probe(candidate.candidate.Path)
		identity := recycleBinIdentity(cfg)
		groupKey := identity.key
		if !identity.known {
			groupKey = strings.ToLower(candidate.candidate.Path)
		}
		group, ok := groups[groupKey]
		if !ok {
			group = &recycleBinVolumeGroup{config: cfg}
			groups[groupKey] = group
			volumeOrder = append(volumeOrder, groupKey)
		}
		group.candidates = append(group.candidates, candidate)
		if candidate.candidate.Bytes < 0 || group.totalBytes > int64(^uint64(0)>>1)-candidate.candidate.Bytes {
			group.unsafe = true
		} else {
			group.totalBytes += candidate.candidate.Bytes
		}
		if err != nil || !identity.known || cfg.MaxCapacity < 0 || cfg.CurrentUsage < 0 {
			group.unsafe = true
			continue
		}
		if group.config.NukeOnDelete != cfg.NukeOnDelete || group.config.MaxCapacity != cfg.MaxCapacity || group.config.CurrentUsage != cfg.CurrentUsage {
			group.unsafe = true
		}
	}

	return recycleBinCandidateGroups{byVolume: groups, order: volumeOrder}
}

func executeRecycleBinCandidateGroups(ctx context.Context, opts Options, adapter delete.Adapter, groups recycleBinCandidateGroups, result *Result, completed map[string]bool) {
	// Volume grouping is the safety model (aggregate capacity). Progress only
	// marks each category when its candidates are first touched in this phase.
	// A category completes only after every volume that holds its candidates
	// has finished (success, skip, or capacity rejection).
	remainingByCategory := make(map[string]int)
	for _, volume := range groups.order {
		for _, candidate := range groups.byVolume[volume].candidates {
			if candidate.rule != "" {
				remainingByCategory[candidate.rule]++
			}
		}
	}
	reportedCategory := make(map[string]bool)
	reportRecycleCategory := func(candidates []actionExecutionCandidate) {
		for _, candidate := range candidates {
			id := candidate.rule
			if id == "" || reportedCategory[id] {
				continue
			}
			reportedCategory[id] = true
			reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseRecycleBinOperations, id)
		}
	}
	finishVolumeCategories := func(candidates []actionExecutionCandidate) {
		if result == nil {
			return
		}
		finished := make(map[string]bool)
		for _, candidate := range candidates {
			id := candidate.rule
			if id == "" {
				continue
			}
			remainingByCategory[id]--
			if remainingByCategory[id] == 0 {
				finished[id] = true
			}
		}
		for id := range finished {
			outcome := ProjectProvisionalCategoryOutcome(id, *result)
			reportCategoryCompletion(opts.ProgressReporter, ExecutionPhaseRecycleBinOperations, outcome, completed)
		}
	}

	for _, volume := range groups.order {
		group := groups.byVolume[volume]
		switch {
		case group.unsafe:
			reportRecycleCategory(group.candidates)
			skipRecycleBinVolume(result, group.candidates, recycleBinCapacityProbeFailedIssueCode, "Recycle Bin capacity state is unknown; skipping this volume rather than risking permanent deletion")
			finishVolumeCategories(group.candidates)
			continue
		case group.config.NukeOnDelete:
			reportRecycleCategory(group.candidates)
			skipRecycleBinVolume(result, group.candidates, recycleBinDisabledIssueCode, "Recycle Bin is disabled for this volume; items would be permanently deleted")
			finishVolumeCategories(group.candidates)
			continue
		case group.config.CurrentUsage > group.config.MaxCapacity:
			// Recycle Bin already over capacity: nothing fits, skip the volume.
			reportRecycleCategory(group.candidates)
			skipRecycleBinVolume(result, group.candidates, recycleBinCapacityIssueCode, "Recycle Bin is already over capacity for this volume")
			finishVolumeCategories(group.candidates)
			continue
		}

		// Partial fill: move as many candidates as fit the remaining Recycle Bin
		// capacity, preferring smaller candidates first to maximize the count moved,
		// and skip only the overflow. Permanent deletion is never substituted for
		// overflow (ADR 0018). Ties preserve resolve order (defaults before opt-ins)
		// via stable sort, so which candidate fits is deterministic.
		remaining := group.config.MaxCapacity - group.config.CurrentUsage
		fit, overflow := partitionRecycleBinCandidatesByFit(group.candidates, remaining)
		if len(overflow) > 0 {
			reportRecycleCategory(overflow)
			skipRecycleBinVolume(result, overflow, recycleBinCapacityIssueCode, "Selected candidate did not fit the remaining Recycle Bin capacity for this volume and was skipped")
		}
		if len(fit) == 0 {
			finishVolumeCategories(group.candidates)
			continue
		}

		reportRecycleCategory(fit)
		deleteCandidates := make([]delete.Candidate, 0, len(fit))
		byPath := make(map[string]actionExecutionCandidate, len(fit))
		for _, candidate := range fit {
			deleteCandidates = append(deleteCandidates, candidate.candidate)
			byPath[candidate.candidate.Path] = candidate
		}
		preMutation := composeRecycleBinPreMutation(opts, byPath)
		deleteResult := delete.ExecuteWithHooks(ctx, deleteCandidates, adapter, opts.Validator, preMutation)
		for _, item := range deleteResult.Deleted {
			candidate := byPath[item.Path]
			result.Deleted = append(result.Deleted, DeletedItem{
				Path:    item.Path,
				Bytes:   item.Bytes,
				Rule:    candidate.rule,
				Action:  plannedRecycleBinAction,
				IsOptIn: candidate.isOptIn,
			})
		}
		for _, item := range deleteResult.Skipped {
			candidate := byPath[item.Path]
			planned := candidate.plannedAction
			if planned == "" {
				planned = plannedRecycleBinAction
			}
			result.Skipped = append(result.Skipped, SkippedItem{
				Path: item.Path, Bytes: item.Bytes, Rule: candidate.rule,
				PlannedAction: planned,
				Reason:        issue(item.Reason.Code, item.Reason.Message, true, item.Path, candidate.rule),
			})
		}
		finishVolumeCategories(group.candidates)
	}
}

// partitionRecycleBinCandidatesByFit selects the candidates that fit within
// remaining bytes of Recycle Bin capacity, preferring smaller candidates first
// to maximize the count moved. Ties preserve resolve order (defaults before
// opt-ins) via stable sort. Candidates with non-positive bytes always fit.
// Returns (fit, overflow) each in original resolve order.
func partitionRecycleBinCandidatesByFit(candidates []actionExecutionCandidate, remaining int64) (fit, overflow []actionExecutionCandidate) {
	type indexed struct {
		i    int
		size int64
	}
	order := make([]indexed, len(candidates))
	for i, c := range candidates {
		size := c.candidate.Bytes
		if size < 0 {
			size = 0
		}
		order[i] = indexed{i: i, size: size}
	}
	sort.SliceStable(order, func(a, b int) bool { return order[a].size < order[b].size })
	used := int64(0)
	fits := make([]bool, len(candidates))
	for _, e := range order {
		if e.size > remaining-used {
			fits[e.i] = false
			continue
		}
		fits[e.i] = true
		used += e.size
	}
	for i, c := range candidates {
		if fits[i] {
			fit = append(fit, c)
		} else {
			overflow = append(overflow, c)
		}
	}
	return fit, overflow
}

// executePermanentCandidates runs authorized permanent removal only through the
// permanent remover. Missing authorization skips with a stable reason and never
// reroutes work to the Recycle Bin. Local failures do not block siblings.
//
// Immediate pre-mutation composition (per candidate): PathSafe, then optional
// category-owned identity validation, then the shared permanent remover.
// Category rejection is a recoverable skip that keeps delete_permanently as the
// planned action and never falls back to the Recycle Bin.
func executePermanentCandidates(ctx context.Context, opts Options, candidates []actionExecutionCandidate, result *Result, completed map[string]bool) {
	if len(candidates) == 0 {
		return
	}
	permanentAction := string(PlannedActionDeletePermanently)
	// Progress at category boundaries only (first-seen rule order). Mutation
	// order and safety model stay candidate-list order within each category.
	// Each category has one planned action, so finishing its permanent batch
	// is a full mid-flight completion boundary.
	groups := groupCandidatesByCategory(candidates)
	if !opts.AllowPermanentDeletion {
		for _, group := range groups {
			reportExecutionProgress(opts.ProgressReporter, ExecutionPhasePermanentOperations, group.rule)
			for _, candidate := range group.candidates {
				result.Skipped = append(result.Skipped, SkippedItem{
					Path:          candidate.candidate.Path,
					Bytes:         candidate.candidate.Bytes,
					Rule:          candidate.rule,
					PlannedAction: permanentAction,
					Reason: issue(
						permanentDeletionNotAuthorizedIssueCode,
						"permanent deletion is not authorized for this run; planned action is unchanged",
						true,
						candidate.candidate.Path,
						candidate.rule,
					),
				})
			}
			if result != nil {
				outcome := ProjectProvisionalCategoryOutcome(group.rule, *result)
				reportCategoryCompletion(opts.ProgressReporter, ExecutionPhasePermanentOperations, outcome, completed)
			}
		}
		return
	}

	remover := opts.PermanentRemover
	if remover == nil {
		remover = delete.FilesystemPermanentRemover{}
	}

	for _, group := range groups {
		reportExecutionProgress(opts.ProgressReporter, ExecutionPhasePermanentOperations, group.rule)
		deleteCandidates := make([]delete.Candidate, 0, len(group.candidates))
		byPath := make(map[string]actionExecutionCandidate, len(group.candidates))
		for _, candidate := range group.candidates {
			deleteCandidates = append(deleteCandidates, candidate.candidate)
			byPath[candidate.candidate.Path] = candidate
		}

		preMutation := composePermanentPreMutation(opts, byPath)
		permanentResult := delete.ExecutePermanentDetailed(ctx, deleteCandidates, remover, opts.Validator, preMutation)
		for _, item := range permanentResult.Items {
			candidate := byPath[item.Path]
			switch item.Kind {
			case delete.PermanentOutcomeDeleted:
				result.Deleted = append(result.Deleted, DeletedItem{
					Path:    item.Path,
					Bytes:   item.Bytes,
					Rule:    candidate.rule,
					Action:  permanentAction,
					IsOptIn: candidate.isOptIn,
				})
			case delete.PermanentOutcomePartial:
				// Continue-on-error permanent removal deleted some files and
				// skipped the rest (e.g. locked by another process). The deleted
				// portion counts as successful permanent bytes; the remaining
				// measured bytes are recorded as a failed portion so the category
				// projects to partial and the partial-risk notice still applies.
				if item.Bytes > 0 {
					result.Deleted = append(result.Deleted, DeletedItem{
						Path:    item.Path,
						Bytes:   item.Bytes,
						Rule:    candidate.rule,
						Action:  permanentAction,
						IsOptIn: candidate.isOptIn,
					})
				}
				remainder := candidate.candidate.Bytes - item.Bytes
				if remainder < 0 {
					remainder = 0
				}
				result.Failed = append(result.Failed, FailedItem{
					Path:          item.Path,
					Bytes:         remainder,
					Rule:          candidate.rule,
					PlannedAction: permanentAction,
					Action:        permanentAction,
					Reason:        issue(permanentDeletePartialIssueCode, item.Reason.Message, true, item.Path, candidate.rule),
				})
			case delete.PermanentOutcomeFailed:
				result.Failed = append(result.Failed, FailedItem{
					Path:          item.Path,
					Bytes:         item.Bytes,
					Rule:          candidate.rule,
					PlannedAction: permanentAction,
					Action:        permanentAction,
					Reason:        issue(permanentDeleteFailedIssueCode, item.Reason.Message, true, item.Path, candidate.rule),
				})
			case delete.PermanentOutcomeCanceled:
				result.Skipped = append(result.Skipped, SkippedItem{
					Path:          item.Path,
					Bytes:         item.Bytes,
					Rule:          candidate.rule,
					PlannedAction: permanentAction,
					Reason:        issue(item.Reason.Code, item.Reason.Message, true, item.Path, candidate.rule),
				})
			default:
				// Pre-mutation skips (protection, reparse, hardlink, category identity).
				result.Skipped = append(result.Skipped, SkippedItem{
					Path:          item.Path,
					Bytes:         item.Bytes,
					Rule:          candidate.rule,
					PlannedAction: permanentAction,
					Reason:        issue(item.Reason.Code, item.Reason.Message, true, item.Path, candidate.rule),
				})
			}
		}
		if result != nil {
			outcome := ProjectProvisionalCategoryOutcome(group.rule, *result)
			reportCategoryCompletion(opts.ProgressReporter, ExecutionPhasePermanentOperations, outcome, completed)
		}
	}
}

// categoryCandidateGroup is one rule's candidates in first-seen order among the
// partitioned permanent/recycle lists. Used only for progress boundaries and
// permanent per-category mutation batching; it does not authorize work.
type categoryCandidateGroup struct {
	rule       string
	candidates []actionExecutionCandidate
}

func groupCandidatesByCategory(candidates []actionExecutionCandidate) []categoryCandidateGroup {
	if len(candidates) == 0 {
		return nil
	}
	indexByRule := make(map[string]int, len(candidates))
	groups := make([]categoryCandidateGroup, 0)
	for _, candidate := range candidates {
		rule := candidate.rule
		if idx, ok := indexByRule[rule]; ok {
			groups[idx].candidates = append(groups[idx].candidates, candidate)
			continue
		}
		indexByRule[rule] = len(groups)
		groups = append(groups, categoryCandidateGroup{
			rule:       rule,
			candidates: []actionExecutionCandidate{candidate},
		})
	}
	return groups
}

// composePermanentPreMutation builds the optional delete-layer hook that
// dispatches to category-owned validators. Returns nil when no candidate has a
// registered validator so existing PathSafe-only categories keep the same
// shared remover path with no extra hook call.
func composePermanentPreMutation(opts Options, byPath map[string]actionExecutionCandidate) delete.PreMutationValidator {
	hasValidator := false
	for _, candidate := range byPath {
		if lookupPermanentIdentityValidator(opts, candidate.rule) != nil {
			hasValidator = true
			break
		}
	}
	if !hasValidator {
		return nil
	}
	return func(candidate delete.Candidate) (pathsafe.Reason, bool) {
		meta, ok := byPath[candidate.Path]
		if !ok {
			// Unexpected path: fail closed without mutation.
			return pathsafe.Reason{
				Code:    "identity_mismatch",
				Message: "permanent candidate context is unavailable for identity validation",
			}, false
		}
		validator := lookupPermanentIdentityValidator(opts, meta.rule)
		if validator == nil {
			return pathsafe.Reason{}, true
		}
		return validator(PermanentIdentityCandidate{
			Path:             candidate.Path,
			Bytes:            candidate.Bytes,
			Category:         meta.rule,
			residueDiscovery: opts.GrokBuildResidueDiscoveryOptions,
		})
	}
}

// composeRecycleBinPreMutation builds the optional delete-layer hook that
// dispatches to action-neutral category-owned identity validators immediately
// before each Recycle Bin move. Returns nil when no candidate has a registered
// validator so existing PathSafe-only categories keep the same shared adapter
// path with no extra hook call. A rejection is a recoverable skip that keeps the
// move_to_recycle_bin planned action and never reroutes to permanent deletion.
func composeRecycleBinPreMutation(opts Options, byPath map[string]actionExecutionCandidate) delete.PreMutationValidator {
	hasValidator := false
	for _, candidate := range byPath {
		if lookupCategoryIdentityValidator(opts, candidate.rule) != nil {
			hasValidator = true
			break
		}
	}
	if !hasValidator {
		return nil
	}
	return func(candidate delete.Candidate) (pathsafe.Reason, bool) {
		meta, ok := byPath[candidate.Path]
		if !ok {
			// Unexpected path: fail closed without mutation.
			return pathsafe.Reason{
				Code:    "identity_mismatch",
				Message: "recycle bin candidate context is unavailable for identity validation",
			}, false
		}
		validator := lookupCategoryIdentityValidator(opts, meta.rule)
		if validator == nil {
			return pathsafe.Reason{}, true
		}
		return validator(CategoryIdentityCandidate{
			Path:                                candidate.Path,
			Bytes:                               candidate.Bytes,
			Category:                            meta.rule,
			PlannedAction:                       meta.plannedAction,
			nvidiaDiscovery:                     opts.NVIDIAInstallerCacheDiscoveryOptions,
			lghubDiscovery:                      opts.LGHUBCacheDiscoveryOptions,
			thunderUpdateDownloadDiscovery:      opts.ThunderUpdateDownloadDiscoveryOptions,
			windowsTempDiscovery:                opts.WindowsTempDiscoveryOptions,
			windowsUpdateDownloadCacheDiscovery: opts.WindowsUpdateDownloadCacheDiscoveryOptions,
			validator:                           opts.Validator,
		})
	}
}

func recycleBinIdentity(config RecycleBinVolumeConfig) recycleBinVolumeIdentity {
	key := strings.ToLower(strings.TrimSpace(config.Volume))
	return recycleBinVolumeIdentity{key: key, known: key != ""}
}

func skipRecycleBinVolume(result *Result, candidates []actionExecutionCandidate, code, message string) {
	for _, candidate := range candidates {
		planned := candidate.plannedAction
		if planned == "" {
			planned = plannedRecycleBinAction
		}
		result.Skipped = append(result.Skipped, SkippedItem{
			Path: candidate.candidate.Path, Bytes: candidate.candidate.Bytes, Rule: candidate.rule,
			PlannedAction: planned,
			Reason:        issue(code, message, true, candidate.candidate.Path, candidate.rule),
		})
	}
}
