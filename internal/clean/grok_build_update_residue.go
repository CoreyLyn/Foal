package clean

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// CategoryGrokBuildUpdateResidue is the canonical opt-in category for abandoned
// Windows Grok Build updater executable backups under $GROK_HOME\bin.
const CategoryGrokBuildUpdateResidue = "grok-build-update-residue"

// ApplicationGrokBuild is one logical process identity covering both grok.exe
// and agent.exe (distinctive-process idle-before-and-after gate).
const ApplicationGrokBuild = "grok_build"

// grokBuildUpdateResidueOptInImpactNotice is path-free impact vocabulary when
// Opt-in candidates are present. Residue files are abandoned updater backups;
// current executables and downloads are never candidates.
const grokBuildUpdateResidueOptInImpactNotice = "Opt-in Grok Build update residue cleanup permanently deletes abandoned Windows updater backups (*.exe.old). Current Grok binaries, downloads, sessions, credentials, and configuration are not removed. Permanent deletion is ordinary filesystem removal (not secure erasure)."

// grokBuildUpdateQuietWindow is the fail-closed recent-update witness window.
// Matches Grok's own stale-download boundary and exceeds its internal download timeout.
const grokBuildUpdateQuietWindow = time.Hour

// GrokBuildResidueDiscoveryOptions injects environment, home, and clock seams
// for hermetic tests. Production leaves the zero value so process env and
// time.Now are used. Tests must never inspect the real user .grok profile.
type GrokBuildResidueDiscoveryOptions struct {
	// LookupEnv overrides os.LookupEnv when non-nil.
	LookupEnv func(string) (string, bool)
	// UserHomeDir overrides os.UserHomeDir when non-nil.
	UserHomeDir func() (string, error)
	// Now overrides the current time used by the update-quiet witness gate.
	// Zero means time.Now().
	Now time.Time
}

type grokBuildResidueDeps struct {
	lookupEnv   func(string) (string, bool)
	userHomeDir func() (string, error)
	joinPath    func(...string) string
	lstat       func(string) (os.FileInfo, error)
	readDir     func(string) ([]os.DirEntry, error)
	now         func() time.Time
}

func productionGrokBuildResidueDeps(opts GrokBuildResidueDiscoveryOptions) grokBuildResidueDeps {
	deps := grokBuildResidueDeps{
		lookupEnv:   os.LookupEnv,
		userHomeDir: os.UserHomeDir,
		joinPath:    filepath.Join,
		lstat:       os.Lstat,
		readDir:     os.ReadDir,
		now:         time.Now,
	}
	if opts.LookupEnv != nil {
		deps.lookupEnv = opts.LookupEnv
	}
	if opts.UserHomeDir != nil {
		deps.userHomeDir = opts.UserHomeDir
	}
	if !opts.Now.IsZero() {
		now := opts.Now
		deps.now = func() time.Time { return now }
	}
	return deps
}

// categoryResolverGrokBuildUpdateResidue is the dedicated private resolver kind
// for this product-scoped residue category. It is intentionally not a
// developer-cache or application-cache entry so dev-caches does not select it.
const categoryResolverGrokBuildUpdateResidue categoryResolverKind = "grok-build-update-residue"

type grokBuildUpdateResidueResolver struct{}

func (grokBuildUpdateResidueResolver) resolve(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	resolveGrokBuildUpdateResidueCategory(ctx, opts, category, core)
}

func grokBuildUpdateResidueCategoryEntry(definition CleanupCategoryDefinition) categoryCatalogEntry {
	return categoryCatalogEntry{
		definition:          definition,
		resolverKind:        categoryResolverGrokBuildUpdateResidue,
		resolver:            grokBuildUpdateResidueResolver{},
		cliAgentProduct:     true, // participates in `cli-agents` selection group only
		runningApplications: []string{ApplicationGrokBuild},
	}
}

func init() {
	registerPermanentIdentityValidator(CategoryGrokBuildUpdateResidue, validateGrokBuildUpdateResidueIdentity)
}

// resolveGrokBuildUpdateResidueCategory is the shared Dry-run / Execute / TUI
// resolution seam for exact Grok Build updater residue.
//
// Order: resolve safe root → pre idle gate → update-quiet witness → discover
// exact files → post idle gate → measure independent ordinary-file candidates.
// Missing root/bin is silent empty. Invalid override, unreadable safety state,
// running/unknown process, or recent witness skips the whole category.
func resolveGrokBuildUpdateResidueCategory(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	if core == nil || category != CategoryGrokBuildUpdateResidue {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	deps := productionGrokBuildResidueDeps(opts.GrokBuildResidueDiscoveryOptions)

	root, rootStatus := resolveGrokBuildHome(deps, opts.Validator)
	switch rootStatus {
	case grokRootSilentAbsence:
		return
	case grokRootInvalidOverride, grokRootUnreadable:
		// Fail closed: no candidates, no CWD/default fallback after invalid override.
		core.Diagnostics = append(core.Diagnostics, issue(
			"grok_home_unavailable",
			"Grok home could not be resolved safely; Grok Build update residue was skipped",
			true,
			"",
			category,
		))
		return
	case grokRootProtected:
		// Protected root suppresses path-backed discovery before totals.
		if root != "" {
			core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, root)
		}
		return
	case grokRootOK:
		// continue
	default:
		return
	}

	binDir := deps.joinPath(root, "bin")
	// Missing bin is silent absence (no incomplete diagnostic).
	if !isGrokSafeDirectory(deps, binDir) {
		// Distinguish missing vs unreadable: missing → empty; unreadable → skip.
		if _, err := deps.lstat(binDir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			core.Diagnostics = append(core.Diagnostics, issue(
				"grok_bin_unreadable",
				"Grok bin directory could not be inspected safely; Grok Build update residue was skipped",
				true,
				"",
				category,
			))
		}
		return
	}

	// Distinctive-process idle-before-and-after. Without a detector, fail open
	// for process state only (matches other distinctive-process categories so
	// hermetic path tests can run); production CLI always injects detection.
	hasDetector := opts.DetectRunningApplications != nil
	var preStates []RunningApplicationState
	if hasDetector {
		preStates = opts.DetectRunningApplications(ctx)
		idle, projected, reason := observeDevCacheApps(
			[]string{ApplicationGrokBuild},
			binDir,
			category,
			preStates,
		)
		// Project Grok identity so CLI/TUI can report path-free running state.
		runningGateOutcome{runningStates: projected}.apply(&core.RunningStates, nil)
		if !idle {
			if reason != nil {
				// Whole-category skip: use bin as the path-bearing skip anchor.
				core.Skipped = append(core.Skipped, SkippedItem{
					Path:          binDir,
					Bytes:         0,
					Rule:          category,
					PlannedAction: plannedActionForOpts(opts, category),
					Reason:        *reason,
				})
			}
			return
		}
	}

	// Update-quiet witness on downloads\grok-* (never candidates).
	quiet, witnessDiag := grokUpdateWitnessQuiet(deps, root)
	if witnessDiag != nil {
		core.Diagnostics = append(core.Diagnostics, *witnessDiag)
		return
	}
	if !quiet {
		core.Skipped = append(core.Skipped, SkippedItem{
			Path:          deps.joinPath(root, "downloads"),
			Bytes:         0,
			Rule:          category,
			PlannedAction: plannedActionForOpts(opts, category),
			Reason: issue(
				"grok_update_recent",
				"Recent or unproven Grok update activity was observed; Grok Build update residue was skipped",
				true,
				deps.joinPath(root, "downloads"),
				category,
			),
		})
		return
	}

	select {
	case <-ctx.Done():
		core.Diagnostics = append(core.Diagnostics, issue("context_canceled", ctx.Err().Error(), true, binDir, category))
		return
	default:
	}

	candidates := discoverGrokBuildUpdateResidueFiles(deps, root, binDir)
	if len(candidates) == 0 {
		return
	}

	// Protection: suppress protected files before totals; siblings remain independent.
	var surviving []string
	for _, path := range candidates {
		if opts.Validator.IsUserProtected(path) {
			core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, path)
			continue
		}
		// Also suppress when the root/bin is protected (defensive; root check above).
		if opts.Validator.IsUserProtected(binDir) || opts.Validator.IsUserProtected(root) {
			core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, path)
			continue
		}
		surviving = append(surviving, path)
	}
	if len(surviving) == 0 {
		return
	}

	// Measure independent ordinary-file candidates.
	type measured struct {
		path  string
		bytes int64
	}
	var measuredCandidates []measured
	for _, path := range surviving {
		select {
		case <-ctx.Done():
			core.Diagnostics = append(core.Diagnostics, issue("context_canceled", ctx.Err().Error(), true, path, category))
			return
		default:
		}
		// Re-confirm ordinary non-reparse file immediately before measuring.
		if !isGrokSafeOrdinaryFile(deps, path) {
			continue
		}
		bytes, err := measureBytes(ctx, path)
		if err != nil {
			if ctx.Err() != nil {
				core.Diagnostics = append(core.Diagnostics, issue("context_canceled", ctx.Err().Error(), true, path, category))
				return
			}
			continue
		}
		if bytes <= 0 {
			continue
		}
		measuredCandidates = append(measuredCandidates, measured{path: path, bytes: bytes})
	}
	if len(measuredCandidates) == 0 {
		return
	}

	// Post idle re-check: discard every measured candidate when Grok starts.
	if hasDetector {
		postStates := opts.DetectRunningApplications(ctx)
		idle, projected, reason := observeDevCacheApps(
			[]string{ApplicationGrokBuild},
			binDir,
			category,
			postStates,
		)
		runningGateOutcome{runningStates: projected}.apply(&core.RunningStates, nil)
		if !idle {
			if reason != nil {
				core.Skipped = append(core.Skipped, SkippedItem{
					Path:          binDir,
					Bytes:         0,
					Rule:          category,
					PlannedAction: plannedActionForOpts(opts, category),
					Reason:        *reason,
				})
			}
			return
		}
	}

	for _, c := range measuredCandidates {
		core.OptInCandidates = append(core.OptInCandidates, OptInCandidate{
			Path:          c.path,
			Bytes:         c.bytes,
			Category:      category,
			PlannedAction: plannedActionForOpts(opts, category),
		})
	}
}

// grokRootStatus classifies Grok home resolution outcomes.
type grokRootStatus int

const (
	grokRootOK grokRootStatus = iota
	grokRootSilentAbsence
	grokRootInvalidOverride
	grokRootUnreadable
	grokRootProtected
)

// resolveGrokBuildHome resolves the Grok home root.
//
// Unset GROK_HOME → current user's .grok (USERPROFILE or home).
// Non-blank absolute safe override → accepted.
// Blank, relative, unsafe, protected, or reparse roots fail closed without
// default/CWD fallback. Missing default root is silent absence.
func resolveGrokBuildHome(deps grokBuildResidueDeps, validator pathsafe.Validator) (string, grokRootStatus) {
	if deps.lookupEnv == nil {
		deps.lookupEnv = os.LookupEnv
	}
	if deps.userHomeDir == nil {
		deps.userHomeDir = os.UserHomeDir
	}
	if deps.joinPath == nil {
		deps.joinPath = filepath.Join
	}
	if deps.lstat == nil {
		deps.lstat = os.Lstat
	}

	raw, set := deps.lookupEnv("GROK_HOME")
	if set {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			// Blank/whitespace override: fail closed, no default fallback.
			return "", grokRootInvalidOverride
		}
		if !filepath.IsAbs(trimmed) {
			return "", grokRootInvalidOverride
		}
		if strings.HasPrefix(filepath.Clean(trimmed), `\\`) {
			return "", grokRootInvalidOverride
		}
		cleaned := filepath.Clean(trimmed)
		if !isGrokHomePathSafe(cleaned) {
			return "", grokRootInvalidOverride
		}
		if validator.IsUserProtected(cleaned) {
			return cleaned, grokRootProtected
		}
		info, err := deps.lstat(cleaned)
		if errors.Is(err, fs.ErrNotExist) {
			// Override points at missing path: silent absence (not default fallback).
			return "", grokRootSilentAbsence
		}
		if err != nil {
			return "", grokRootUnreadable
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", grokRootInvalidOverride
		}
		return cleaned, grokRootOK
	}

	// Unset: current-user .grok only.
	var home string
	if profile, ok := deps.lookupEnv("USERPROFILE"); ok {
		if trimmed := strings.TrimSpace(profile); trimmed != "" && filepath.IsAbs(trimmed) {
			home = trimmed
		}
	}
	if home == "" {
		h, err := deps.userHomeDir()
		if err != nil || strings.TrimSpace(h) == "" {
			return "", grokRootSilentAbsence
		}
		home = h
	}
	root := deps.joinPath(filepath.Clean(home), ".grok")
	if !filepath.IsAbs(root) || !isGrokHomePathSafe(root) {
		return "", grokRootSilentAbsence
	}
	if validator.IsUserProtected(root) {
		return root, grokRootProtected
	}
	info, err := deps.lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return "", grokRootSilentAbsence
	}
	if err != nil {
		return "", grokRootUnreadable
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		// Default root is a reparse/file: fail closed as unreadable safety state.
		return "", grokRootUnreadable
	}
	return root, grokRootOK
}

// isGrokHomePathSafe rejects volume roots and well-known system trees so an
// override cannot authorize scanning Windows or Program Files.
func isGrokHomePathSafe(path string) bool {
	cleaned := strings.ToLower(filepath.Clean(path))
	if cleaned == "" || !filepath.IsAbs(cleaned) {
		return false
	}
	if strings.HasPrefix(cleaned, `\\`) {
		return false
	}
	// Volume root (C:\).
	vol := strings.ToLower(filepath.VolumeName(cleaned))
	if vol != "" {
		rest := strings.TrimPrefix(cleaned, vol)
		rest = strings.Trim(rest, `/\`)
		if rest == "" {
			return false
		}
	}
	for _, protected := range []string{`c:\windows`, `c:\program files`, `c:\program files (x86)`} {
		if cleaned == protected || strings.HasPrefix(cleaned, protected+string(filepath.Separator)) {
			return false
		}
	}
	return true
}

// isGrokBuildUpdateResidueFileName reports whether basename is an exact
// lowercase Grok updater backup form. Case variants, generic .old, rollback
// backups, and malformed numeric fields fail closed.
func isGrokBuildUpdateResidueFileName(name string) bool {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return false
	}
	// Exact fixed names.
	if name == "grok.exe.old" || name == "agent.exe.old" {
		return true
	}
	// Anchored generated: <base>.exe.old.<pid>-<seq>.old
	for _, base := range []string{"grok.exe.old.", "agent.exe.old."} {
		if !strings.HasPrefix(name, base) {
			continue
		}
		rest := strings.TrimPrefix(name, base)
		if !strings.HasSuffix(rest, ".old") {
			return false
		}
		mid := strings.TrimSuffix(rest, ".old")
		// mid must be <pid>-<seq> with non-empty decimal digits on both sides.
		dash := strings.IndexByte(mid, '-')
		if dash <= 0 || dash == len(mid)-1 {
			return false
		}
		pid, seq := mid[:dash], mid[dash+1:]
		if !isNonEmptyDecimalDigits(pid) || !isNonEmptyDecimalDigits(seq) {
			return false
		}
		// No extra dashes or junk: only one dash separating pid and seq.
		if strings.ContainsRune(seq, '-') {
			return false
		}
		return true
	}
	return false
}

func isNonEmptyDecimalDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// discoverGrokBuildUpdateResidueFiles returns absolute paths of exact ordinary
// non-reparse residue files directly under binDir. Order is deterministic
// (case-insensitive name sort). Nested paths and non-matching siblings are excluded.
func discoverGrokBuildUpdateResidueFiles(deps grokBuildResidueDeps, root, binDir string) []string {
	if deps.readDir == nil {
		deps.readDir = os.ReadDir
	}
	if deps.lstat == nil {
		deps.lstat = os.Lstat
	}
	if deps.joinPath == nil {
		deps.joinPath = filepath.Join
	}
	entries, err := deps.readDir(binDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if !isGrokBuildUpdateResidueFileName(name) {
			continue
		}
		names = append(names, name)
	}
	sort.SliceStable(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	var out []string
	for _, name := range names {
		path := deps.joinPath(binDir, name)
		// Direct child only (reject path tricks).
		if !isDirectChildPath(binDir, path) {
			continue
		}
		// Must still sit under the resolved root\bin.
		if !isPathUnderRoot(root, path) {
			continue
		}
		if !isGrokSafeOrdinaryFile(deps, path) {
			continue
		}
		out = append(out, path)
	}
	return out
}

// grokUpdateWitnessQuiet inspects $GROK_HOME\downloads for direct ordinary
// grok-* files used only as update-activity witnesses. Missing downloads is
// quiet. Unreadable existing directory or unknown/recent/future relevant time
// is not quiet. Witnesses never become candidates.
func grokUpdateWitnessQuiet(deps grokBuildResidueDeps, root string) (quiet bool, diagnostic *StructuredIssue) {
	if deps.lstat == nil {
		deps.lstat = os.Lstat
	}
	if deps.readDir == nil {
		deps.readDir = os.ReadDir
	}
	if deps.joinPath == nil {
		deps.joinPath = filepath.Join
	}
	if deps.now == nil {
		deps.now = time.Now
	}
	downloads := deps.joinPath(root, "downloads")
	info, err := deps.lstat(downloads)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		issue := issue(
			"grok_downloads_unreadable",
			"Grok downloads directory could not be inspected; Grok Build update residue was skipped",
			true,
			"",
			CategoryGrokBuildUpdateResidue,
		)
		return false, &issue
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		issue := issue(
			"grok_downloads_unreadable",
			"Grok downloads path is not a safe directory; Grok Build update residue was skipped",
			true,
			"",
			CategoryGrokBuildUpdateResidue,
		)
		return false, &issue
	}
	entries, err := deps.readDir(downloads)
	if err != nil {
		issue := issue(
			"grok_downloads_unreadable",
			"Grok downloads directory could not be listed; Grok Build update residue was skipped",
			true,
			"",
			CategoryGrokBuildUpdateResidue,
		)
		return false, &issue
	}
	now := deps.now()
	for _, entry := range entries {
		name := entry.Name()
		// Witness prefix is lowercase grok-; case variants are ignored as witnesses
		// (only exact lowercase activity markers count — false skips acceptable).
		if !strings.HasPrefix(name, "grok-") {
			continue
		}
		path := deps.joinPath(downloads, name)
		if !isDirectChildPath(downloads, path) {
			continue
		}
		fi, err := deps.lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			// Unreadable relevant metadata fails closed.
			return false, nil
		}
		// Only ordinary non-reparse files are witnesses.
		if fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
			continue
		}
		mod := fi.ModTime()
		if mod.IsZero() {
			// Unknown relevant time → not proven quiet.
			return false, nil
		}
		// Future timestamp is not proven quiet.
		if mod.After(now) {
			return false, nil
		}
		// Within one hour of now → recent update activity.
		if now.Sub(mod) < grokBuildUpdateQuietWindow {
			return false, nil
		}
		// Exactly at boundary (now.Sub(mod) == 1h) is quiet: "within one hour"
		// is exclusive of the boundary so only strictly recent times skip.
		// Plan: "exactly-at-boundary" is a quiet case in the test matrix.
	}
	return true, nil
}

func isGrokSafeDirectory(deps grokBuildResidueDeps, path string) bool {
	if deps.lstat == nil {
		deps.lstat = os.Lstat
	}
	info, err := deps.lstat(path)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return info.IsDir()
}

func isGrokSafeOrdinaryFile(deps grokBuildResidueDeps, path string) bool {
	if deps.lstat == nil {
		deps.lstat = os.Lstat
	}
	info, err := deps.lstat(path)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return info.Mode().IsRegular()
}

func isPathUnderRoot(root, path string) bool {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	if cleanRoot == "" || cleanPath == "" {
		return false
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// validateGrokBuildUpdateResidueIdentity is the category-owned permanent
// pre-mutation check (#261). Re-resolves the safe root and requires:
// exact allowlisted basename, direct parent = root\bin, ordinary non-reparse
// file type. Protection is enforced by PathSafe before this hook. Never mutates
// or expands candidates.
func validateGrokBuildUpdateResidueIdentity(candidate PermanentIdentityCandidate) (pathsafe.Reason, bool) {
	if candidate.Category != CategoryGrokBuildUpdateResidue {
		return pathsafe.Reason{
			Code:    "identity_mismatch",
			Message: "category identity does not match Grok Build update residue",
		}, false
	}
	path := candidate.Path
	if strings.TrimSpace(path) == "" {
		return pathsafe.Reason{Code: "identity_mismatch", Message: "empty Grok residue path"}, false
	}

	deps := productionGrokBuildResidueDeps(candidate.residueDiscovery)
	// Fresh root resolve (same injects as discovery when Execute filled them).
	root, status := resolveGrokBuildHome(deps, pathsafe.Validator{})
	if status != grokRootOK {
		return pathsafe.Reason{
			Code:    "identity_mismatch",
			Message: "Grok home is no longer resolvable for residue identity validation",
		}, false
	}
	binDir := deps.joinPath(root, "bin")
	base := filepath.Base(path)
	if !isGrokBuildUpdateResidueFileName(base) {
		return pathsafe.Reason{
			Code:    "identity_mismatch",
			Message: "basename is not an exact Grok Build updater residue name",
		}, false
	}
	// Direct parent must be the resolved bin directory.
	parent := filepath.Clean(filepath.Dir(path))
	if !pathIdentityEqual(parent, binDir) {
		return pathsafe.Reason{
			Code:    "identity_mismatch",
			Message: "residue file is not a direct child of the resolved Grok bin directory",
		}, false
	}
	// Exact stored basename path under bin.
	expected := deps.joinPath(binDir, base)
	if !pathIdentityEqual(path, expected) {
		return pathsafe.Reason{
			Code:    "identity_mismatch",
			Message: "residue path does not match the exact allowlisted bin child",
		}, false
	}
	if !isGrokSafeOrdinaryFile(deps, path) {
		return pathsafe.Reason{
			Code:    "identity_mismatch",
			Message: "residue path is not an ordinary non-reparse file",
		}, false
	}
	return pathsafe.Reason{}, true
}

func pathIdentityEqual(a, b string) bool {
	return pathsafe.NormalizePathForIdentity(a) == pathsafe.NormalizePathForIdentity(b)
}
