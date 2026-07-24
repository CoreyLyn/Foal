package clean

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// categoryResolverFixedRoot is the shared private resolver kind for exact
// fixed-root machine-wide (and similar) opt-in categories. Policy data drives
// root resolution, child acceptance, stability windows, optional activity
// gates, and impact notices; discovery walks through one engine so each
// category is a row, not a clone.
const categoryResolverFixedRoot categoryResolverKind = "fixed-root"

// fixedRootActivityStatus is the three-state observation used by fixed-root
// category-wide process/service gates. Unknown never means idle.
type fixedRootActivityStatus string

const (
	fixedRootActivityIdle    fixedRootActivityStatus = "idle"
	fixedRootActivityRunning fixedRootActivityStatus = "running"
	fixedRootActivityUnknown fixedRootActivityStatus = "unknown"
)

// fixedRootActivityState is one conservative process/service observation.
// Message is optional path-free diagnostic text.
type fixedRootActivityState struct {
	Status  fixedRootActivityStatus
	Message string
}

// fixedRootPolicy is the data + small-predicate registration for one fixed-root
// category. Callers outside the engine never see this type.
type fixedRootPolicy struct {
	id string
	// resolveRoot derives the production discovery root from discoveryContext
	// (env / fixed path). Empty ok means silent absence. Root overrides in the
	// context short-circuit this function.
	resolveRoot func(dc discoveryContext) (root string, ok bool)
	// acceptChild reports whether a direct child name/info may become a candidate
	// before stability and measurement. Nil accepts every non-reparse child.
	acceptChild func(name string, info os.FileInfo) bool
	// stabilityDays is the minimum inclusive age in whole days of the latest
	// observed modification across the child and safely inspected descendants.
	// Zero means no stability window.
	stabilityDays int
	// requireInspection forces deep inspectOpportunity even when stabilityDays
	// is zero (to obtain bytes). Mutually exclusive with bytesFromFileSize for
	// current policies.
	requireInspection bool
	// bytesFromFileSize measures reclaimable bytes from Lstat size (ordinary
	// files only). When true, discovery and revalidation skip deep inspection.
	bytesFromFileSize bool
	// requireByteMatch revalidates that the live size still equals candidate.Bytes
	// (LGHUB content-addressed blobs).
	requireByteMatch bool
	// detectActivity is the production activity detector. Nil means no
	// category-wide activity gate. Tests override via discoveryContext.
	detectActivity func(context.Context) fixedRootActivityState
	// activityApplication is the logical process/service identity projected into
	// RunningStates when detectActivity is set.
	activityApplication string
	// activitySkipCode / activityRunningMessage / activityUnknownMessage are the
	// whole-category SkippedItem reason when the gate fails closed.
	activitySkipCode       string
	activityRunningMessage string
	activityUnknownMessage string
	// rootUnreadableCode / rootUnreadableMessage are the stable diagnostic when
	// the root exists but cannot be inspected or listed.
	rootUnreadableCode    string
	rootUnreadableMessage string
	// revalidationCode is the pathsafe reason code for failed pre-mutation
	// identity checks.
	revalidationCode string
	// impactNotice is the path-free opt-in impact vocabulary.
	impactNotice string
	// requireExactOnly / requireRecycleBin are catalog validation expectations.
	requireExactOnly  bool
	requireRecycleBin bool
}

var fixedRootPolicies = map[string]fixedRootPolicy{}

func registerFixedRootPolicy(p fixedRootPolicy) {
	if p.id == "" {
		panic("fixed-root policy missing id")
	}
	if _, exists := fixedRootPolicies[p.id]; exists {
		panic("duplicate fixed-root policy " + p.id)
	}
	fixedRootPolicies[p.id] = p
}

func fixedRootPolicyByID(id string) (fixedRootPolicy, bool) {
	p, ok := fixedRootPolicies[id]
	return p, ok
}

type fixedRootResolver struct{}

func (fixedRootResolver) resolve(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	resolveFixedRootCategory(ctx, opts, category, core)
}

func fixedRootCategoryEntry(definition CleanupCategoryDefinition) categoryCatalogEntry {
	return categoryCatalogEntry{
		definition:   definition,
		resolverKind: categoryResolverFixedRoot,
		resolver:     fixedRootResolver{},
	}
}

// resolveFixedRootCategory is the shared DryRun / ResolveCategory seam for every
// fixed-root policy. It must not mutate. Gates fail closed; missing roots are
// silent absence; unreadable root enumeration is a whole-category diagnostic.
//
// Order: resolve root → root shape → protection → optional pre activity gate →
// discover exact candidates → optional post activity gate → surface candidates.
func resolveFixedRootCategory(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	if core == nil {
		return
	}
	policy, ok := fixedRootPolicyByID(category)
	if !ok {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dc := discoveryContextFromOptions(opts)
	root, rootOK := resolveFixedRoot(dc, policy)
	if !rootOK || strings.TrimSpace(root) == "" {
		return
	}

	info, err := dc.lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		core.Diagnostics = append(core.Diagnostics, issue(policy.rootUnreadableCode,
			policy.rootUnreadableMessage, true, "", category))
		return
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 || !info.IsDir() {
		core.Diagnostics = append(core.Diagnostics, issue(policy.rootUnreadableCode,
			policy.rootUnreadableMessage, true, "", category))
		return
	}
	if opts.Validator.IsUserProtected(root) {
		core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, root)
		return
	}

	if policy.detectActivity != nil || dc.hasActivityDetector(category) {
		if !fixedRootActivityIdleGate(ctx, dc, policy, root, category, core) {
			return
		}
	}

	entries, err := dc.readDir(root)
	if err != nil {
		core.Diagnostics = append(core.Diagnostics, issue(policy.rootUnreadableCode,
			policy.rootUnreadableMessage, true, "", category))
		return
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.SliceStable(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })

	type measuredCandidate struct {
		path  string
		bytes int64
	}
	var measured []measuredCandidate

	for _, name := range names {
		select {
		case <-ctx.Done():
			core.Diagnostics = append(core.Diagnostics, issue(PreviewReasonContextCanceled, ctx.Err().Error(), true, "", category))
			return
		default:
		}

		path := dc.joinPath(root, name)
		if !isDirectChildPath(root, path) {
			continue
		}
		childInfo, statErr := dc.lstat(path)
		if statErr != nil {
			continue
		}
		if childInfo.Mode()&os.ModeSymlink != 0 || childInfo.Mode()&os.ModeIrregular != 0 {
			continue
		}
		if policy.acceptChild != nil && !policy.acceptChild(name, childInfo) {
			continue
		}
		if opts.Validator.IsUserProtected(path) {
			core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, path)
			continue
		}

		var bytes int64
		switch {
		case policy.bytesFromFileSize:
			if !childInfo.Mode().IsRegular() {
				continue
			}
			bytes = childInfo.Size()
		case policy.requireInspection || policy.stabilityDays > 0:
			inspection, inspectErr := inspectOpportunity(ctx, path, userTempDescendantLimit, dc.walkDir)
			if inspectErr != nil {
				continue
			}
			if policy.stabilityDays > 0 {
				if int(dc.now.Sub(inspection.latestModifiedAt)/(24*time.Hour)) < policy.stabilityDays {
					continue
				}
			}
			bytes = inspection.bytes
		}

		measured = append(measured, measuredCandidate{path: path, bytes: bytes})
	}

	if len(measured) == 0 {
		return
	}

	if policy.detectActivity != nil || dc.hasActivityDetector(category) {
		if !fixedRootActivityIdleGate(ctx, dc, policy, root, category, core) {
			return
		}
	}

	for _, c := range measured {
		core.OptInCandidates = append(core.OptInCandidates, OptInCandidate{
			Path:          c.path,
			Bytes:         c.bytes,
			Category:      category,
			PlannedAction: plannedActionForOpts(opts, category),
		})
	}
}

func resolveFixedRoot(dc discoveryContext, policy fixedRootPolicy) (string, bool) {
	if root, ok := dc.categoryRootOverride(policy.id); ok {
		return root, true
	}
	if policy.resolveRoot == nil {
		return "", false
	}
	return policy.resolveRoot(dc)
}

// fixedRootActivityIdleGate detects activity once and returns true only when idle.
// Running or unknown records a whole-category SkippedItem and projects the
// application running state. Clean never stops processes or services.
func fixedRootActivityIdleGate(ctx context.Context, dc discoveryContext, policy fixedRootPolicy, root, category string, core *categoryCoreResult) bool {
	detect := dc.activityDetectorFor(category)
	if detect == nil {
		detect = policy.detectActivity
	}
	if detect == nil {
		return true
	}
	activity := detect(ctx)
	app := policy.activityApplication
	switch activity.Status {
	case fixedRootActivityIdle:
		if app != "" {
			runningGateOutcome{runningStates: []RunningApplicationState{{
				Application: app,
				State:       RunningApplicationStateIdle,
			}}}.apply(&core.RunningStates, nil)
		}
		return true
	case fixedRootActivityRunning:
		if app != "" {
			core.RunningStates = mergeRunningApplicationStates(core.RunningStates, RunningApplicationState{
				Application: app,
				State:       RunningApplicationStateRunning,
				Message:     activity.Message,
			})
		}
		core.Skipped = append(core.Skipped, SkippedItem{
			Path:          root,
			Bytes:         0,
			Rule:          category,
			PlannedAction: plannedActionForCategory(category),
			Reason:        issue(policy.activitySkipCode, policy.activityRunningMessage, true, root, category),
		})
		return false
	default:
		if app != "" {
			core.RunningStates = mergeRunningApplicationStates(core.RunningStates, RunningApplicationState{
				Application: app,
				State:       RunningApplicationStateUnknown,
				Message:     activity.Message,
			})
		}
		core.Skipped = append(core.Skipped, SkippedItem{
			Path:          root,
			Bytes:         0,
			Rule:          category,
			PlannedAction: plannedActionForCategory(category),
			Reason:        issue(policy.activitySkipCode, policy.activityUnknownMessage, true, root, category),
		})
		return false
	}
}

// validateFixedRootIdentity is the shared action-neutral pre-mutation check for
// fixed-root categories. It re-resolves the root, requires a direct ordinary
// child, re-applies acceptChild / stability / size policy, and never repeats the
// category-wide activity gate.
func validateFixedRootIdentity(candidate CategoryIdentityCandidate) (pathsafe.Reason, bool) {
	policy, ok := fixedRootPolicyByID(candidate.Category)
	if !ok {
		return pathsafe.Reason{Code: "identity_mismatch", Message: "category is not a fixed-root category"}, false
	}
	reject := func(message string) (pathsafe.Reason, bool) {
		code := policy.revalidationCode
		if code == "" {
			code = "fixed_root_revalidation_failed"
		}
		return pathsafe.Reason{Code: code, Message: message}, false
	}
	path := strings.TrimSpace(candidate.Path)
	if path == "" {
		return reject("fixed-root candidate path is empty")
	}

	dc := discoveryContextFromFixedRootOptions(candidate.fixedRootDiscovery)

	root, rootOK := resolveFixedRoot(dc, policy)
	if !rootOK || strings.TrimSpace(root) == "" {
		return reject("fixed-root discovery root is no longer resolvable")
	}
	if !isDirectChildPath(root, path) {
		return reject("fixed-root candidate is not a direct child of the discovery root")
	}
	info, err := dc.lstat(path)
	if err != nil {
		return reject("fixed-root candidate is no longer readable")
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
		return reject("fixed-root candidate is no longer an ordinary non-reparse path")
	}
	if policy.acceptChild != nil && !policy.acceptChild(filepath.Base(path), info) {
		return reject("fixed-root candidate no longer matches the child policy")
	}
	if policy.bytesFromFileSize {
		if !info.Mode().IsRegular() {
			return reject("fixed-root candidate is no longer an ordinary non-reparse file")
		}
		if policy.requireByteMatch && info.Size() != candidate.Bytes {
			return reject("fixed-root candidate size changed since resolution; it was not moved to the Recycle Bin")
		}
		return pathsafe.Reason{}, true
	}
	if policy.stabilityDays > 0 || policy.requireInspection {
		inspection, inspectErr := inspectOpportunity(context.Background(), path, userTempDescendantLimit, dc.walkDir)
		if inspectErr != nil {
			return reject("fixed-root candidate could not be re-inspected")
		}
		if policy.stabilityDays > 0 {
			if int(dc.now.Sub(inspection.latestModifiedAt)/(24*time.Hour)) < policy.stabilityDays {
				return reject("fixed-root candidate is no longer past the stability window")
			}
		}
	}
	return pathsafe.Reason{}, true
}

// validateFixedRootRegistryEntry checks catalog metadata against the registered
// policy for a fixed-root category.
func validateFixedRootRegistryEntry(entry categoryCatalogEntry) error {
	id := entry.definition.Identifier
	policy, ok := fixedRootPolicyByID(id)
	if !ok {
		return fmt.Errorf("fixed-root category %q has no registered policy", id)
	}
	if entry.definition.Eligibility != CategoryEligibilityOptIn {
		return fmt.Errorf("fixed-root category %q must use opt-in eligibility", id)
	}
	if policy.requireRecycleBin && entry.definition.PlannedAction != PlannedActionMoveToRecycleBin {
		return fmt.Errorf("fixed-root category %q must declare move_to_recycle_bin", id)
	}
	if policy.requireExactOnly && entry.definition.SelectionPolicy != CategorySelectionPolicyExactOnly {
		return fmt.Errorf("fixed-root category %q must be exact-selection-only", id)
	}
	if entry.cliAgentProduct {
		return fmt.Errorf("fixed-root category %q must not be a cli-agent product", id)
	}
	return nil
}
