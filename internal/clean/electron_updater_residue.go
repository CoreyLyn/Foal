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

// CategoryElectronUpdaterResidue is the canonical opt-in category for stale
// electron-builder updater payloads under per-application directories in
// LOCALAPPDATA whose names end with "-updater".
const CategoryElectronUpdaterResidue = "electron-updater-residue"

// electronUpdaterResidueOptInImpactNotice is the path-free impact vocabulary
// shown when candidates are present.
const electronUpdaterResidueOptInImpactNotice = "Opt-in Electron updater residue cleanup moves stale installer payloads to the Recycle Bin. Payloads can normally be re-downloaded on demand, but this may re-download installers for recently updated applications. This is not permanent deletion and is not secure erasure."

// ElectronUpdaterResidueDiscoveryOptions injects root override, clock, and
// other seams for hermetic tests. Production leaves this as the zero value.
type ElectronUpdaterResidueDiscoveryOptions struct {
	// Root overrides LOCALAPPDATA for testing.
	Root string
	// Now overrides time.Now() for quiet window testing.
	Now time.Time
}

type electronUpdaterResidueDeps struct {
	root            string
	now             func() time.Time
	lstat           func(string) (os.FileInfo, error)
	readDir         func(string) ([]fs.DirEntry, error)
	joinPath        func(...string) string
	isPathUnderRoot func(string, string) bool
}

func productionElectronUpdaterResidueDeps(opts ElectronUpdaterResidueDiscoveryOptions) electronUpdaterResidueDeps {
	deps := electronUpdaterResidueDeps{
		now:             time.Now,
		lstat:           os.Lstat,
		readDir:         os.ReadDir,
		joinPath:        filepath.Join,
		isPathUnderRoot: isPathUnderRoot,
	}
	if trimmed := strings.TrimSpace(opts.Root); trimmed != "" {
		deps.root = trimmed
	} else {
		deps.root = os.Getenv("LOCALAPPDATA")
	}
	if !opts.Now.IsZero() {
		now := opts.Now
		deps.now = func() time.Time { return now }
	}
	return deps
}

const categoryResolverElectronUpdaterResidue categoryResolverKind = "electron-updater-residue"

type electronUpdaterResidueResolver struct{}

func (electronUpdaterResidueResolver) resolve(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	resolveElectronUpdaterResidueCategory(ctx, opts, category, core)
}

func electronUpdaterResidueCategoryEntry(definition CleanupCategoryDefinition) categoryCatalogEntry {
	return categoryCatalogEntry{
		definition:        definition,
		resolverKind:      categoryResolverElectronUpdaterResidue,
		resolver:          electronUpdaterResidueResolver{},
		previewSafetyNote: staticPreviewSafetyNote(electronUpdaterResidueOptInImpactNotice),
	}
}

func init() {
	registerCategoryIdentityValidator(CategoryElectronUpdaterResidue, validateElectronUpdaterResidueIdentity)
}

// validatedElectronUpdaterDir is the structural proof of one matched *-updater
// directory. allowlistedFiles is every conforming allowlisted file in the
// directory (used for the per-directory quiet window and for identity
// revalidation membership). candidateFiles is the subset that becomes opt-in
// candidates: update-info.json is a candidate only alongside a sibling payload
// .exe in pending (a pending containing only update-info.json yields no
// candidates), per docs/plan/electron-updater-residue.md.
type validatedElectronUpdaterDir struct {
	path             string
	allowlistedFiles []string
	candidateFiles   []string
	isAllowedFile    func(string) bool
}

// validateElectronUpdaterResidueIdentity is the action-neutral, category-owned
// immediate pre-mutation validator. Immediately before Recycle Bin move, it
// repeats the structural proof against a fresh read and confirms the candidate
// is still an allowlisted file in a conforming directory with unchanged size.
func validateElectronUpdaterResidueIdentity(candidate CategoryIdentityCandidate) (pathsafe.Reason, bool) {
	reject := func(message string) (pathsafe.Reason, bool) {
		return pathsafe.Reason{Code: "electron_updater_residue_revalidation_failed", Message: message}, false
	}
	if candidate.Category != CategoryElectronUpdaterResidue {
		return pathsafe.Reason{Code: "identity_mismatch", Message: "category identity does not match electron-updater-residue"}, false
	}
	candidatePath := strings.TrimSpace(candidate.Path)
	if candidatePath == "" {
		return reject("Electron updater residue candidate path is empty")
	}

	deps := productionElectronUpdaterResidueDeps(candidate.electronUpdaterResidueDiscovery)
	if strings.TrimSpace(deps.root) == "" {
		return reject("LOCALAPPDATA is no longer resolvable")
	}

	// Find the updater directory that contains the candidate.
	updaterDir := findUpdaterDirForCandidate(deps, candidatePath)
	if updaterDir == "" {
		return reject("Electron updater residue candidate is not within an updater directory")
	}

	// Revalidate the entire structure against a fresh read.
	validated, ok := validateElectronUpdaterDir(deps, updaterDir)
	if !ok {
		return reject("Electron updater directory no longer matches the structural allowlist")
	}
	if !validated.isAllowedFile(candidatePath) {
		return reject("Electron updater residue candidate is no longer in the structural allowlist")
	}

	// Verify the candidate file still exists, is an ordinary non-reparse file,
	// and has not changed size since resolution.
	info, err := deps.lstat(candidatePath)
	if err != nil {
		return reject("Electron updater residue candidate is no longer readable")
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 || !info.Mode().IsRegular() {
		return reject("Electron updater residue candidate is no longer an ordinary non-reparse file")
	}
	if info.Size() != candidate.Bytes {
		return reject("Electron updater residue candidate size changed since resolution; it was not moved to the Recycle Bin")
	}

	return pathsafe.Reason{}, true
}

// findUpdaterDirForCandidate walks up from the candidate to find the direct
// child of LOCALAPPDATA whose name ends with "-updater".
func findUpdaterDirForCandidate(deps electronUpdaterResidueDeps, candidatePath string) string {
	currentPath := candidatePath
	for {
		parent := filepath.Dir(currentPath)
		if pathIdentityEqual(parent, deps.root) {
			name := filepath.Base(currentPath)
			if strings.HasSuffix(strings.ToLower(name), "-updater") {
				return currentPath
			}
			return ""
		}
		if pathIdentityEqual(parent, currentPath) {
			return ""
		}
		name := filepath.Base(parent)
		if strings.HasSuffix(strings.ToLower(name), "-updater") {
			grandparent := filepath.Dir(parent)
			if pathIdentityEqual(grandparent, deps.root) {
				return parent
			}
		}
		currentPath = parent
	}
}

// validateElectronUpdaterDir proves one *-updater directory conforms to the
// structural allowlist (fail closed on ANY unknown child) and returns the
// allowlisted files plus the candidate subset.
func validateElectronUpdaterDir(deps electronUpdaterResidueDeps, dir string) (validatedElectronUpdaterDir, bool) {
	entries, err := deps.readDir(dir)
	if err != nil {
		return validatedElectronUpdaterDir{}, false
	}

	var allowlistedFiles []string
	var candidateFiles []string
	isAllowedFile := make(map[string]bool)

	addFile := func(path string, isCandidate bool) {
		allowlistedFiles = append(allowlistedFiles, path)
		isAllowedFile[path] = true
		if isCandidate {
			candidateFiles = append(candidateFiles, path)
		}
	}

	for _, entry := range entries {
		name := entry.Name()
		lowerName := strings.ToLower(name)

		switch lowerName {
		case "installer.exe", "current.blockmap":
			info, err := entry.Info()
			if err != nil {
				return validatedElectronUpdaterDir{}, false
			}
			if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 || !info.Mode().IsRegular() {
				return validatedElectronUpdaterDir{}, false
			}
			addFile(deps.joinPath(dir, name), true)

		case "pending":
			if !entry.IsDir() {
				return validatedElectronUpdaterDir{}, false
			}
			info, err := entry.Info()
			if err != nil {
				return validatedElectronUpdaterDir{}, false
			}
			if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
				return validatedElectronUpdaterDir{}, false
			}

			pendingDir := deps.joinPath(dir, name)
			pendingEntries, err := deps.readDir(pendingDir)
			if err != nil {
				return validatedElectronUpdaterDir{}, false
			}

			type pendingFile struct {
				name string
				path string
			}
			var validatedPending []pendingFile
			hasExePayload := false
			for _, pendingEntry := range pendingEntries {
				pendingName := pendingEntry.Name()
				pendingLower := strings.ToLower(pendingName)
				isExe := strings.HasSuffix(pendingLower, ".exe")
				switch pendingLower {
				case "update-info.json", "current.blockmap":
					// allowlisted metadata
				default:
					if !isExe {
						// Unknown child - fail closed.
						return validatedElectronUpdaterDir{}, false
					}
				}
				pendingInfo, err := pendingEntry.Info()
				if err != nil {
					return validatedElectronUpdaterDir{}, false
				}
				if pendingInfo.Mode()&os.ModeSymlink != 0 || pendingInfo.Mode()&os.ModeIrregular != 0 || !pendingInfo.Mode().IsRegular() {
					return validatedElectronUpdaterDir{}, false
				}
				if isExe {
					hasExePayload = true
				}
				validatedPending = append(validatedPending, pendingFile{pendingName, deps.joinPath(pendingDir, pendingName)})
			}

			for _, pf := range validatedPending {
				pfLower := strings.ToLower(pf.name)
				isCandidate := pfLower != "update-info.json" || hasExePayload
				addFile(pf.path, isCandidate)
			}

		default:
			// Unknown child - fail closed.
			return validatedElectronUpdaterDir{}, false
		}
	}

	return validatedElectronUpdaterDir{
		path:             dir,
		allowlistedFiles: allowlistedFiles,
		candidateFiles:   candidateFiles,
		isAllowedFile: func(path string) bool {
			return isAllowedFile[path]
		},
	}, true
}

// resolveElectronUpdaterResidueCategory is the shared DryRun / ResolveCategory
// resolution seam.
func resolveElectronUpdaterResidueCategory(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	if core == nil || category != CategoryElectronUpdaterResidue {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	deps := productionElectronUpdaterResidueDeps(opts.ElectronUpdaterResidueDiscoveryOptions)

	root := deps.root
	if strings.TrimSpace(root) == "" {
		return
	}

	info, err := deps.lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		core.Diagnostics = append(core.Diagnostics, issue("electron_updater_root_unreadable",
			"LOCALAPPDATA could not be inspected; electron-updater-residue was skipped", true, "", category))
		return
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 || !info.IsDir() {
		return
	}

	entries, err := deps.readDir(root)
	if err != nil {
		return
	}

	var updaterDirs []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), "-updater") {
			continue
		}
		if !entry.IsDir() {
			continue
		}
		dirInfo, err := entry.Info()
		if err != nil {
			continue
		}
		if dirInfo.Mode()&os.ModeSymlink != 0 || dirInfo.Mode()&os.ModeIrregular != 0 {
			continue
		}
		dirPath := deps.joinPath(root, name)
		if !isDirectChildPath(root, dirPath) {
			continue
		}
		updaterDirs = append(updaterDirs, dirPath)
	}

	sort.SliceStable(updaterDirs, func(i, j int) bool {
		return strings.ToLower(updaterDirs[i]) < strings.ToLower(updaterDirs[j])
	})

	for _, updaterDir := range updaterDirs {
		select {
		case <-ctx.Done():
			core.Diagnostics = append(core.Diagnostics, issue(PreviewReasonContextCanceled, ctx.Err().Error(), true, "", category))
			return
		default:
		}

		if opts.Validator.IsUserProtected(updaterDir) {
			core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, updaterDir)
			continue
		}

		validated, ok := validateElectronUpdaterDir(deps, updaterDir)
		if !ok {
			continue
		}

		// Per-directory quiet window: any allowlisted file written within 24h,
		// in the future, or with unreadable metadata skips the whole directory.
		var latestModTime time.Time
		skipDir := false
		for _, filePath := range validated.allowlistedFiles {
			fileInfo, err := deps.lstat(filePath)
			if err != nil {
				core.Skipped = append(core.Skipped, SkippedItem{
					Path:          updaterDir,
					Bytes:         0,
					Rule:          category,
					PlannedAction: plannedActionForOpts(opts, category),
					Reason: issue("electron_update_recent",
						"Electron updater directory has unreadable file metadata; skipped",
						true, updaterDir, category),
				})
				skipDir = true
				break
			}
			modTime := fileInfo.ModTime()
			if modTime.IsZero() || modTime.After(deps.now()) {
				core.Skipped = append(core.Skipped, SkippedItem{
					Path:          updaterDir,
					Bytes:         0,
					Rule:          category,
					PlannedAction: plannedActionForOpts(opts, category),
					Reason: issue("electron_update_recent",
						"Electron updater directory has files with future or unknown timestamps; skipped",
						true, updaterDir, category),
				})
				skipDir = true
				break
			}
			if modTime.After(latestModTime) {
				latestModTime = modTime
			}
		}
		if skipDir {
			continue
		}
		if deps.now().Sub(latestModTime) < 24*time.Hour {
			core.Skipped = append(core.Skipped, SkippedItem{
				Path:          updaterDir,
				Bytes:         0,
				Rule:          category,
				PlannedAction: plannedActionForOpts(opts, category),
				Reason: issue("electron_update_recent",
					"Electron updater directory has files written within the last 24 hours; skipped",
					true, updaterDir, category),
			})
			continue
		}

		for _, filePath := range validated.candidateFiles {
			if opts.Validator.IsUserProtected(filePath) {
				core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, filePath)
				continue
			}
			fileInfo, err := deps.lstat(filePath)
			if err != nil {
				continue
			}
			core.OptInCandidates = append(core.OptInCandidates, OptInCandidate{
				Path:          filePath,
				Bytes:         fileInfo.Size(),
				Category:      category,
				PlannedAction: plannedActionForOpts(opts, category),
			})
		}
	}
}
