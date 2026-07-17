package clean

import (
	"context"
	"strings"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/delete"
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
	reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseScanning)

	// Phase 1: skeleton / plan
	if opts.ProtectionLoadError != nil {
		result := protectionLoadFailure("execute", opts, start)
		reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseComplete)
		return result
	}
	categoryPlan := effectiveCategoryPlan(opts)
	result := newCleanResultSkeleton("execute", opts)

	// Phase 2: fresh resolve defaults + selected opt-ins
	executionCandidates := resolveExecuteCandidates(ctx, opts, categoryPlan, &result)

	// Phase 3: partition Recycle Bin vs permanent
	recycleBinCandidates, permanentCandidates := partitionByPlannedAction(executionCandidates)

	// Phase 4: aggregate Recycle Bin capacity preflight (RB only)
	reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseRecycleBinSafety)
	executionGroups := prepareRecycleBinCandidateGroups(opts, recycleBinCandidates)

	// Phase 5: mutate Recycle Bin first
	reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseRecycleBinOperations)
	adapter := opts.RecycleBinAdapter
	if adapter == nil {
		adapter = delete.WindowsRecycleBinAdapter{}
	}
	executeRecycleBinCandidateGroups(ctx, opts, adapter, executionGroups, &result)

	// Phase 6: mutate permanent last
	if len(permanentCandidates) > 0 {
		reportExecutionProgress(opts.ProgressReporter, ExecutionPhasePermanentOperations)
		executePermanentCandidates(ctx, opts, permanentCandidates, &result)
	}

	// Phase 7: history + completion
	result.ElapsedMS = time.Since(start).Milliseconds()
	result.Totals = totals(result)
	recordHistorySession(ctx, opts, result, start, time.Now())
	reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseComplete)
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
		if candidate.plannedAction == string(DeletionActionDeletePermanently) {
			permanent = append(permanent, candidate)
			continue
		}
		recycleBin = append(recycleBin, candidate)
	}
	return recycleBin, permanent
}

func reportExecutionProgress(reporter ProgressReporter, phase ExecutionPhase) {
	if reporter == nil {
		return
	}
	defer func() { _ = recover() }()
	reporter(ExecutionProgress{Phase: phase})
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

func executeRecycleBinCandidateGroups(ctx context.Context, opts Options, adapter delete.Adapter, groups recycleBinCandidateGroups, result *Result) {
	for _, volume := range groups.order {
		group := groups.byVolume[volume]
		switch {
		case group.unsafe:
			skipRecycleBinVolume(result, group.candidates, recycleBinCapacityProbeFailedIssueCode, "Recycle Bin capacity state is unknown; skipping this volume rather than risking permanent deletion")
			continue
		case group.config.NukeOnDelete:
			skipRecycleBinVolume(result, group.candidates, recycleBinDisabledIssueCode, "Recycle Bin is disabled for this volume; items would be permanently deleted")
			continue
		case group.config.CurrentUsage > group.config.MaxCapacity || group.totalBytes > group.config.MaxCapacity-group.config.CurrentUsage:
			skipRecycleBinVolume(result, group.candidates, recycleBinCapacityIssueCode, "Selected candidates exceed the remaining Recycle Bin capacity for this volume")
			continue
		}

		deleteCandidates := make([]delete.Candidate, 0, len(group.candidates))
		byPath := make(map[string]actionExecutionCandidate, len(group.candidates))
		for _, candidate := range group.candidates {
			deleteCandidates = append(deleteCandidates, candidate.candidate)
			byPath[candidate.candidate.Path] = candidate
		}
		deleteResult := delete.ExecuteWithValidator(ctx, deleteCandidates, adapter, opts.Validator)
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
	}
}

// executePermanentCandidates runs authorized permanent removal only through the
// permanent remover. Missing authorization skips with a stable reason and never
// reroutes work to the Recycle Bin. Local failures do not block siblings.
func executePermanentCandidates(ctx context.Context, opts Options, candidates []actionExecutionCandidate, result *Result) {
	if len(candidates) == 0 {
		return
	}
	permanentAction := string(DeletionActionDeletePermanently)
	if !opts.AllowPermanentDeletion {
		for _, candidate := range candidates {
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
		return
	}

	remover := opts.PermanentRemover
	if remover == nil {
		remover = delete.FilesystemPermanentRemover{}
	}

	deleteCandidates := make([]delete.Candidate, 0, len(candidates))
	byPath := make(map[string]actionExecutionCandidate, len(candidates))
	for _, candidate := range candidates {
		deleteCandidates = append(deleteCandidates, candidate.candidate)
		byPath[candidate.candidate.Path] = candidate
	}

	permanentResult := delete.ExecutePermanentWithValidator(ctx, deleteCandidates, remover, opts.Validator)
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
			// Pre-mutation skips (protection, reparse, hardlink, validation).
			result.Skipped = append(result.Skipped, SkippedItem{
				Path:          item.Path,
				Bytes:         item.Bytes,
				Rule:          candidate.rule,
				PlannedAction: permanentAction,
				Reason:        issue(item.Reason.Code, item.Reason.Message, true, item.Path, candidate.rule),
			})
		}
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
