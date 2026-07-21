package clean

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// CategoryNVIDIAInstallerCache is the canonical exact-selection-only opt-in
// category for strictly validated, completed legacy NVIDIA display-driver
// download-task directories under the fixed legacy Downloader root. Evidence is
// Not proven: the Planned action is always move_to_recycle_bin and permanent
// deletion is never eligible. See docs/research/nvidia-installer-downloader-cache.md.
const CategoryNVIDIAInstallerCache = "nvidia_installer_cache"

// ApplicationNVIDIA is the one logical process/service identity Foal reports for
// NVIDIA application, container, helper, overlay, installer, and service
// activity. Clean never stops these; active or unknown state skips the whole
// nvidia_installer_cache category.
const ApplicationNVIDIA = "nvidia"

// nvidiaDownloaderRoot is the only production discovery root. Foal never uses
// environment, registry, alternate-drive, or parent-directory discovery.
const nvidiaDownloaderRoot = `C:\ProgramData\NVIDIA Corporation\Downloader`

// nvidiaInstallerCacheOptInImpactNotice is the path-free Not-proven impact
// vocabulary shown when a candidate is present. It discloses re-download and
// lost offline install/rollback convenience without promising rollback is
// unaffected and without implying permanent-delete eligibility.
const nvidiaInstallerCacheOptInImpactNotice = "Opt-in NVIDIA installer cache cleanup moves a verified completed NVIDIA driver download to the Recycle Bin. The driver can normally be downloaded again, but an offline install or rollback may require a new download. NVIDIA update activity is never stopped, and uncertain or active tasks are skipped. This is not permanent deletion and is not secure erasure."

// Stable NVIDIA skip/diagnostic reason codes. Codes never embed candidate paths
// or raw operating-system messages.
const (
	nvidiaActivitySkipCode       = PreviewReasonNVIDIAActivity
	nvidiaRootUnreadableCode     = "nvidia_downloader_unreadable"
	nvidiaStatusUnreadableCode   = "nvidia_status_registry_unreadable"
	nvidiaRevalidationFailedCode = "nvidia_installer_cache_revalidation_failed"
	nvidiaLegacyCompletedStatus  = 2
	nvidiaDisplayDriverType      = 1
)

// nvidiaStatusMaxBytes bounds status.json and gfeupdate.json parsing so a
// malformed or hostile registry cannot exhaust memory.
const nvidiaStatusMaxBytes = 8 << 20 // 8 MiB

// nvidiaRecentWriteWindow excludes task payloads written within the last hour so
// an active or freshly completed download is never treated as inactive.
const nvidiaRecentWriteWindow = time.Hour

// nvidiaHexTaskIDLength is the exact immediate task-directory name length.
const nvidiaHexTaskIDLength = 32

// nvidiaDownloadHost is the exact NVIDIA-owned download origin host required by
// a task record's HTTPS download URL. Redirects, user info, and other hosts fail closed.
const nvidiaDownloadHost = "download.nvidia.com"

// NVIDIAActivityStatus reports whether relevant NVIDIA process/service activity
// is idle, running, or of unknown/undetermined state. Unknown never means idle.
type NVIDIAActivityStatus string

const (
	NVIDIAActivityIdle    NVIDIAActivityStatus = "idle"
	NVIDIAActivityRunning NVIDIAActivityStatus = "running"
	NVIDIAActivityUnknown NVIDIAActivityStatus = "unknown"
)

// NVIDIAActivityState is one conservative process/service observation. Message
// is optional path-free diagnostic text.
type NVIDIAActivityState struct {
	Status  NVIDIAActivityStatus
	Message string
}

// NVIDIAPayloadForensics is the Windows-specific file identity Foal requires
// before a payload may be a candidate. A payload must be a single hard link with
// no alternate data streams. Non-Windows production leaves this fail-closed.
type NVIDIAPayloadForensics struct {
	// HardLinkCount is the number of hard links to the payload file. Exactly one
	// is required; greater than one means the content is shared elsewhere.
	HardLinkCount uint32
	// HasAlternateDataStreams is true when the payload carries any NTFS alternate
	// data stream. Any alternate stream excludes the task.
	HasAlternateDataStreams bool
}

// NVIDIAInstallerCacheDiscoveryOptions injects the fixed-root override, clock,
// and platform-specific detection seams for hermetic tests. Production leaves
// the zero value so the fixed ProgramData root, time.Now, and the platform
// detectors are used. Tests must never inspect a real NVIDIA Downloader tree.
type NVIDIAInstallerCacheDiscoveryOptions struct {
	// Root overrides the fixed production discovery root. Test-only: production
	// leaves it empty so only C:\ProgramData\NVIDIA Corporation\Downloader is used.
	Root string
	// Now overrides the recent-write clock. Zero means time.Now().
	Now time.Time
	// DetectActivity reports NVIDIA process/service activity. nil selects the
	// production platform detector (processes + services). Non-Windows production
	// fails closed to Unknown so the whole category is skipped.
	DetectActivity func(context.Context) NVIDIAActivityState
	// VerifyPayloadSignature returns nil when path carries a valid Authenticode
	// signature chaining to NVIDIA Corporation. nil selects the production
	// Windows verifier; non-Windows production fails closed.
	VerifyPayloadSignature func(path string) error
	// InspectPayloadForensics returns hard-link count and alternate-data-stream
	// presence for a payload file. nil selects the production Windows inspector;
	// non-Windows production fails closed (unknown).
	InspectPayloadForensics func(path string) (NVIDIAPayloadForensics, error)
}

type nvidiaInstallerCacheDeps struct {
	root             string
	now              func() time.Time
	lstat            func(string) (os.FileInfo, error)
	readDir          func(string) ([]os.DirEntry, error)
	readFileBounded  func(string, int64) ([]byte, error)
	openFile         func(string) (*os.File, error)
	joinPath         func(...string) string
	detectActivity   func(context.Context) NVIDIAActivityState
	verifySignature  func(string) error
	inspectForensics func(string) (NVIDIAPayloadForensics, error)
}

func productionNVIDIAInstallerCacheDeps(opts NVIDIAInstallerCacheDiscoveryOptions) nvidiaInstallerCacheDeps {
	deps := nvidiaInstallerCacheDeps{
		root:             nvidiaDownloaderRoot,
		now:              time.Now,
		lstat:            os.Lstat,
		readDir:          os.ReadDir,
		readFileBounded:  readFileBounded,
		openFile:         os.Open,
		joinPath:         filepath.Join,
		detectActivity:   productionDetectNVIDIAActivity,
		verifySignature:  productionVerifyNVIDIASignature,
		inspectForensics: productionInspectNVIDIAPayloadForensics,
	}
	if trimmed := strings.TrimSpace(opts.Root); trimmed != "" {
		deps.root = filepath.Clean(trimmed)
	}
	if !opts.Now.IsZero() {
		now := opts.Now
		deps.now = func() time.Time { return now }
	}
	if opts.DetectActivity != nil {
		deps.detectActivity = opts.DetectActivity
	}
	if opts.VerifyPayloadSignature != nil {
		deps.verifySignature = opts.VerifyPayloadSignature
	}
	if opts.InspectPayloadForensics != nil {
		deps.inspectForensics = opts.InspectPayloadForensics
	}
	return deps
}

// readFileBounded reads at most max bytes from path. It fails closed when the
// file exceeds the bound so oversized registries are rejected rather than parsed.
func readFileBounded(path string, max int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errors.New("nvidia registry file exceeds bounded size")
	}
	return data, nil
}

// categoryResolverNVIDIAInstallerCache is the dedicated private resolver kind for
// the NVIDIA installer cache. It is intentionally not a developer-cache or
// application-cache entry: the `all`, `dev-caches`, `app-caches`, and
// `cli-agents` tokens never select it, and TUI Select All excludes it.
const categoryResolverNVIDIAInstallerCache categoryResolverKind = "nvidia-installer-cache"

type nvidiaInstallerCacheResolver struct{}

func (nvidiaInstallerCacheResolver) resolve(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	resolveNVIDIAInstallerCacheCategory(ctx, opts, category, core)
}

func nvidiaInstallerCacheCategoryEntry(definition CleanupCategoryDefinition) categoryCatalogEntry {
	return categoryCatalogEntry{
		definition:   definition,
		resolverKind: categoryResolverNVIDIAInstallerCache,
		resolver:     nvidiaInstallerCacheResolver{},
	}
}

func init() {
	registerCategoryIdentityValidator(CategoryNVIDIAInstallerCache, validateNVIDIAInstallerCacheIdentity)
}

// validateNVIDIAInstallerCacheIdentity is the action-neutral, category-owned
// immediate pre-mutation validator (#310). Immediately before the Recycle Bin
// move it repeats the per-task NVIDIA proof against a fresh read of the fixed
// Downloader root: task identity, the unique matching completed display-driver
// record, required fields, the NVIDIA-owned HTTPS URL, single-file containment,
// payload forensics (single hard link, no alternate data streams), the
// recent-write exclusion, active-handoff reference exclusion, checksum integrity,
// the Authenticode signature, metadata stability, filesystem safety, and
// Protection. It never mutates, never expands candidates, and never repeats the
// category-wide process/service idle gate (that belongs to fresh resolution).
// Any changed, ambiguous, protected, unsupported, or invalid state returns
// ok=false so the candidate is skipped fail-closed; the move_to_recycle_bin
// action is preserved and never becomes permanent deletion.
func validateNVIDIAInstallerCacheIdentity(candidate CategoryIdentityCandidate) (pathsafe.Reason, bool) {
	reject := func(message string) (pathsafe.Reason, bool) {
		return pathsafe.Reason{Code: nvidiaRevalidationFailedCode, Message: message}, false
	}
	if candidate.Category != CategoryNVIDIAInstallerCache {
		return pathsafe.Reason{Code: "identity_mismatch", Message: "category identity does not match NVIDIA installer cache"}, false
	}
	taskDir := strings.TrimSpace(candidate.Path)
	if taskDir == "" {
		return reject("NVIDIA installer cache candidate path is empty")
	}

	deps := productionNVIDIAInstallerCacheDeps(candidate.nvidiaDiscovery)
	root := deps.root
	if strings.TrimSpace(root) == "" {
		return reject("NVIDIA Downloader root is no longer resolvable")
	}
	name := filepath.Base(taskDir)
	if !isNVIDIAHexTaskName(name) || !isDirectChildPath(root, taskDir) {
		return reject("NVIDIA installer cache candidate is not a direct 32-hex task directory of the Downloader root")
	}

	// Fresh bounded read of the task registry and active-handoff references.
	opts := Options{
		Validator:                            candidate.validator,
		NVIDIAInstallerCacheDiscoveryOptions: candidate.nvidiaDiscovery,
	}
	var core categoryCoreResult
	statusByTask, ok := parseNVIDIAStatusRegistry(deps, root, candidate.Category, &core)
	if !ok || len(statusByTask) == 0 {
		return reject("NVIDIA task registry is no longer readable or no longer lists a matching task")
	}
	referencedContent := parseNVIDIAReferencedContent(deps, root)

	// Repeat the full single-task evidence chain against the current tree.
	candidatePath, bytes, ok := validateNVIDIATaskDirectory(deps, opts, root, taskDir, name, statusByTask, referencedContent, candidate.Category, &core)
	if !ok {
		return reject("NVIDIA installer cache candidate no longer matches the verified completed driver download; it was not moved to the Recycle Bin")
	}
	if !pathIdentityEqual(candidatePath, taskDir) || bytes != candidate.Bytes {
		return reject("NVIDIA installer cache candidate identity or size changed since resolution; it was not moved to the Recycle Bin")
	}
	return pathsafe.Reason{}, true
}

// nvidiaStatusRecord is the bounded, safety-relevant subset of one legacy
// Downloader status.json task record. Unknown fields are ignored, but every
// field below participates in fail-closed validation.
type nvidiaStatusRecord struct {
	TaskID        string `json:"taskId"`
	Status        *int   `json:"status"`
	DownloadType  *int   `json:"downloadType"`
	Version       string `json:"version"`
	Checksum      string `json:"checksum"`
	FileLocation  string `json:"fileLocation"`
	DownloadURL   string `json:"downloadUrl"`
	ExtractedPath string `json:"extractedPath"`
}

// resolveNVIDIAInstallerCacheCategory is the shared Dry-run / ResolveCategory
// resolution seam. It never mutates. Every gate fails closed: any failure or
// uncertainty produces no candidate, and active or unknown NVIDIA process/service
// state skips the entire category.
//
// Order: resolve fixed root -> pre activity gate -> parse bounded status/gfe
// registries -> per 32-hex task validate (record, type, fields, URL, single-file
// containment, forensics, recent-write, references, checksum, signature) ->
// protection suppression -> post activity gate -> measure candidates.
func resolveNVIDIAInstallerCacheCategory(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	if core == nil || category != CategoryNVIDIAInstallerCache {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	deps := productionNVIDIAInstallerCacheDeps(opts.NVIDIAInstallerCacheDiscoveryOptions)

	root := deps.root
	if strings.TrimSpace(root) == "" {
		return
	}
	// Root must be a real, non-reparse directory. Missing is silent absence.
	info, err := deps.lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		core.Diagnostics = append(core.Diagnostics, issue(nvidiaRootUnreadableCode,
			"NVIDIA Downloader root could not be inspected; NVIDIA installer cache was skipped", true, "", category))
		return
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 || !info.IsDir() {
		core.Diagnostics = append(core.Diagnostics, issue(nvidiaRootUnreadableCode,
			"NVIDIA Downloader root is not an ordinary directory; NVIDIA installer cache was skipped", true, "", category))
		return
	}
	// Protected root suppresses path-backed discovery before totals.
	if opts.Validator.IsUserProtected(root) {
		core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, root)
		return
	}

	// Pre activity gate: active or unknown NVIDIA state skips the whole category.
	if !nvidiaActivityIdleGate(ctx, deps, root, category, core) {
		return
	}

	// Parse the bounded root registries. A malformed/oversized status.json fails
	// closed for the whole category; a missing status.json means no tasks.
	statusByTask, statusOK := parseNVIDIAStatusRegistry(deps, root, category, core)
	if !statusOK {
		return
	}
	if len(statusByTask) == 0 {
		return
	}
	referencedContent := parseNVIDIAReferencedContent(deps, root)

	entries, err := deps.readDir(root)
	if err != nil {
		core.Diagnostics = append(core.Diagnostics, issue(nvidiaRootUnreadableCode,
			"NVIDIA Downloader root could not be listed; NVIDIA installer cache was skipped", true, "", category))
		return
	}

	type measuredCandidate struct {
		path  string
		bytes int64
	}
	var measured []measuredCandidate

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.SliceStable(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })

	for _, name := range names {
		select {
		case <-ctx.Done():
			core.Diagnostics = append(core.Diagnostics, issue(PreviewReasonContextCanceled, ctx.Err().Error(), true, "", category))
			return
		default:
		}
		if !isNVIDIAHexTaskName(name) {
			continue
		}
		taskDir := deps.joinPath(root, name)
		candidatePath, bytes, ok := validateNVIDIATaskDirectory(deps, opts, root, taskDir, name, statusByTask, referencedContent, category, core)
		if !ok {
			continue
		}
		measured = append(measured, measuredCandidate{path: candidatePath, bytes: bytes})
	}

	if len(measured) == 0 {
		return
	}

	// Post activity re-check: any active or unknown NVIDIA state discards all
	// measured candidates for the whole category.
	if !nvidiaActivityIdleGate(ctx, deps, root, category, core) {
		return
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

// nvidiaActivityIdleGate detects NVIDIA activity once and returns true only when
// idle. Running or unknown state records a whole-category SkippedItem and
// projects the NVIDIA running state. Clean never stops processes or services.
func nvidiaActivityIdleGate(ctx context.Context, deps nvidiaInstallerCacheDeps, root, category string, core *categoryCoreResult) bool {
	detect := deps.detectActivity
	if detect == nil {
		detect = productionDetectNVIDIAActivity
	}
	activity := detect(ctx)
	switch activity.Status {
	case NVIDIAActivityIdle:
		runningGateOutcome{runningStates: []RunningApplicationState{{
			Application: ApplicationNVIDIA,
			State:       RunningApplicationStateIdle,
		}}}.apply(&core.RunningStates, nil)
		return true
	case NVIDIAActivityRunning:
		core.RunningStates = mergeRunningApplicationStates(core.RunningStates, RunningApplicationState{
			Application: ApplicationNVIDIA,
			State:       RunningApplicationStateRunning,
			Message:     activity.Message,
		})
		core.Skipped = append(core.Skipped, SkippedItem{
			Path:          root,
			Bytes:         0,
			Rule:          category,
			PlannedAction: plannedActionForCategory(category),
			Reason: issue(nvidiaActivitySkipCode,
				"NVIDIA application, container, helper, overlay, installer, or service is active; NVIDIA installer cache was skipped",
				true, root, category),
		})
		return false
	default:
		core.RunningStates = mergeRunningApplicationStates(core.RunningStates, RunningApplicationState{
			Application: ApplicationNVIDIA,
			State:       RunningApplicationStateUnknown,
			Message:     activity.Message,
		})
		core.Skipped = append(core.Skipped, SkippedItem{
			Path:          root,
			Bytes:         0,
			Rule:          category,
			PlannedAction: plannedActionForCategory(category),
			Reason: issue(nvidiaActivitySkipCode,
				"NVIDIA process or service state could not be determined; NVIDIA installer cache was skipped",
				true, root, category),
		})
		return false
	}
}

// parseNVIDIAStatusRegistry parses the bounded root status.json into a taskId
// map. Returns ok=false (with a diagnostic) when the file exists but is
// unreadable, oversized, malformed, or has duplicate task IDs. A missing
// status.json returns an empty map with ok=true (no tasks, silent absence).
func parseNVIDIAStatusRegistry(deps nvidiaInstallerCacheDeps, root, category string, core *categoryCoreResult) (map[string]nvidiaStatusRecord, bool) {
	statusPath := deps.joinPath(root, "status.json")
	info, err := deps.lstat(statusPath)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]nvidiaStatusRecord{}, true
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		core.Diagnostics = append(core.Diagnostics, issue(nvidiaStatusUnreadableCode,
			"NVIDIA task registry could not be inspected; NVIDIA installer cache was skipped", true, "", category))
		return nil, false
	}
	data, err := deps.readFileBounded(statusPath, nvidiaStatusMaxBytes)
	if err != nil {
		core.Diagnostics = append(core.Diagnostics, issue(nvidiaStatusUnreadableCode,
			"NVIDIA task registry could not be read within safe bounds; NVIDIA installer cache was skipped", true, "", category))
		return nil, false
	}
	records, err := decodeNVIDIAStatusRecords(data)
	if err != nil {
		core.Diagnostics = append(core.Diagnostics, issue(nvidiaStatusUnreadableCode,
			"NVIDIA task registry is not in a recognized format; NVIDIA installer cache was skipped", true, "", category))
		return nil, false
	}
	byTask := make(map[string]nvidiaStatusRecord, len(records))
	duplicate := make(map[string]bool)
	for _, record := range records {
		id := strings.ToLower(strings.TrimSpace(record.TaskID))
		if id == "" {
			continue
		}
		if _, seen := byTask[id]; seen {
			// A duplicate taskId makes the task non-unique: exclude it entirely
			// by marking it duplicate and removing the earlier copy.
			duplicate[id] = true
			continue
		}
		byTask[id] = record
	}
	for id := range duplicate {
		delete(byTask, id)
	}
	return byTask, true
}

// decodeNVIDIAStatusRecords accepts either a top-level JSON array of task records
// or an object with a "tasks" array. Any other shape is rejected fail-closed.
func decodeNVIDIAStatusRecords(data []byte) ([]nvidiaStatusRecord, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, errors.New("empty status registry")
	}
	switch trimmed[0] {
	case '[':
		var records []nvidiaStatusRecord
		if err := json.Unmarshal(data, &records); err != nil {
			return nil, err
		}
		return records, nil
	case '{':
		var wrapper struct {
			Tasks []nvidiaStatusRecord `json:"tasks"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return nil, err
		}
		if wrapper.Tasks == nil {
			return nil, errors.New("status registry object has no tasks array")
		}
		return wrapper.Tasks, nil
	default:
		return nil, errors.New("unrecognized status registry shape")
	}
}

// parseNVIDIAReferencedContent returns the lowercased content of active handoff
// metadata (gfeupdate.json). Any task token appearing in this content is treated
// as a live reference and excluded. Missing or unreadable metadata returns an
// empty string (individual tasks still fail closed on their own gates).
func parseNVIDIAReferencedContent(deps nvidiaInstallerCacheDeps, root string) string {
	gfePath := deps.joinPath(root, "gfeupdate.json")
	info, err := deps.lstat(gfePath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ""
	}
	data, err := deps.readFileBounded(gfePath, nvidiaStatusMaxBytes)
	if err != nil {
		return ""
	}
	return strings.ToLower(string(data))
}

// nvidiaTaskReferenced reports whether the task name appears in referenced active
// handoff content. Broad substring matching accepts frequent false skips.
func nvidiaTaskReferenced(referencedContent, name string) bool {
	if referencedContent == "" {
		return false
	}
	return strings.Contains(referencedContent, strings.ToLower(name))
}

// validateNVIDIATaskDirectory runs the full fail-closed evidence chain for one
// 32-hex task directory. It returns the candidate directory path and measured
// bytes only when every gate passes. Protection suppression is recorded on core
// before totals; all other failures are silent (no candidate).
func validateNVIDIATaskDirectory(
	deps nvidiaInstallerCacheDeps,
	opts Options,
	root, taskDir, name string,
	statusByTask map[string]nvidiaStatusRecord,
	referencedContent string,
	category string,
	core *categoryCoreResult,
) (string, int64, bool) {
	// The task directory must be an ordinary, non-reparse, direct child directory.
	dirInfo, err := deps.lstat(taskDir)
	if err != nil || !isDirectChildPath(root, taskDir) {
		return "", 0, false
	}
	if dirInfo.Mode()&os.ModeSymlink != 0 || dirInfo.Mode()&os.ModeIrregular != 0 || !dirInfo.IsDir() {
		return "", 0, false
	}

	// Exactly one matching, unique record with the accepted completed state and
	// display-driver type.
	record, ok := statusByTask[strings.ToLower(name)]
	if !ok {
		return "", 0, false
	}
	if record.Status == nil || *record.Status != nvidiaLegacyCompletedStatus {
		return "", 0, false
	}
	if record.DownloadType == nil || *record.DownloadType != nvidiaDisplayDriverType {
		return "", 0, false
	}
	if strings.TrimSpace(record.Version) == "" || strings.TrimSpace(record.Checksum) == "" || strings.TrimSpace(record.FileLocation) == "" {
		return "", 0, false
	}
	if strings.TrimSpace(record.ExtractedPath) != "" {
		return "", 0, false
	}
	if !isValidNVIDIADownloadURL(record.DownloadURL) {
		return "", 0, false
	}
	if nvidiaTaskReferenced(referencedContent, name) {
		return "", 0, false
	}

	// fileLocation must canonicalize to a direct regular-file child of the task
	// directory (no traversal, alternate streams, or noncanonical paths).
	payloadPath, ok := canonicalNVIDIAPayloadPath(taskDir, record.FileLocation)
	if !ok {
		return "", 0, false
	}

	// The task directory must contain exactly that one regular file: no
	// subdirectories, extra files, reparse points, or alternate content.
	if !nvidiaDirectoryHoldsOnlyPayload(deps, taskDir, payloadPath) {
		return "", 0, false
	}

	// Payload must be an ordinary non-reparse file with a stable identity.
	payloadInfo, err := deps.lstat(payloadPath)
	if err != nil || payloadInfo.Mode()&os.ModeSymlink != 0 || payloadInfo.Mode()&os.ModeIrregular != 0 || !payloadInfo.Mode().IsRegular() {
		return "", 0, false
	}
	// Recent-write exclusion: unknown, future, or within-window write time fails closed.
	mod := payloadInfo.ModTime()
	now := deps.now()
	if mod.IsZero() || mod.After(now) || now.Sub(mod) < nvidiaRecentWriteWindow {
		return "", 0, false
	}

	// Forensics: exactly one hard link and no alternate data streams.
	forensics, err := deps.inspectForensics(payloadPath)
	if err != nil || forensics.HardLinkCount != 1 || forensics.HasAlternateDataStreams {
		return "", 0, false
	}

	// Integrity fixture: declared checksum must match the payload content.
	if !nvidiaChecksumMatches(deps, payloadPath, record.Checksum) {
		return "", 0, false
	}

	// Authenticode signature chaining to NVIDIA Corporation.
	if err := deps.verifySignature(payloadPath); err != nil {
		return "", 0, false
	}

	// Stability: re-confirm the payload's size and last-write time are unchanged
	// after integrity and signature inspection. Any drift fails closed.
	stableInfo, err := deps.lstat(payloadPath)
	if err != nil || stableInfo.Size() != payloadInfo.Size() || !stableInfo.ModTime().Equal(payloadInfo.ModTime()) {
		return "", 0, false
	}
	if !nvidiaDirectoryHoldsOnlyPayload(deps, taskDir, payloadPath) {
		return "", 0, false
	}

	// Protection: suppress protected candidates before totals. The task directory
	// is the candidate moved to the Recycle Bin.
	if opts.Validator.IsUserProtected(taskDir) || opts.Validator.IsUserProtected(payloadPath) {
		core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, taskDir)
		return "", 0, false
	}

	return taskDir, payloadInfo.Size(), true
}

// isNVIDIAHexTaskName reports whether name is exactly 32 hexadecimal characters.
func isNVIDIAHexTaskName(name string) bool {
	if len(name) != nvidiaHexTaskIDLength {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// isValidNVIDIADownloadURL requires an HTTPS URL on the exact NVIDIA-owned
// download host with no user info, no traversal, and no encoded redirect.
func isValidNVIDIADownloadURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return false
	}
	if parsed.User != nil {
		return false
	}
	if !strings.EqualFold(parsed.Hostname(), nvidiaDownloadHost) {
		return false
	}
	if strings.Contains(parsed.Path, "..") {
		return false
	}
	if parsed.Opaque != "" {
		return false
	}
	return true
}

// canonicalNVIDIAPayloadPath canonicalizes fileLocation and requires it to be a
// direct regular-file child of taskDir. Traversal, alternate streams (":"), and
// noncanonical or out-of-directory paths fail closed.
func canonicalNVIDIAPayloadPath(taskDir, fileLocation string) (string, bool) {
	fileLocation = strings.TrimSpace(fileLocation)
	if fileLocation == "" {
		return "", false
	}
	// Reject NTFS alternate data stream syntax in the declared location. A bare
	// drive letter keeps its colon at index 1; any other colon is a stream
	// separator and fails closed.
	for i := 0; i < len(fileLocation); i++ {
		if fileLocation[i] == ':' && i != 1 {
			return "", false
		}
	}
	if strings.ContainsRune(fileLocation, '\x00') {
		return "", false
	}
	cleaned := filepath.Clean(fileLocation)
	if strings.Contains(cleaned, "..") {
		return "", false
	}
	if !filepath.IsAbs(cleaned) {
		// A bare filename is accepted only as a direct child of the task dir.
		if strings.ContainsAny(fileLocation, `/\`) {
			return "", false
		}
		cleaned = filepath.Join(taskDir, cleaned)
	}
	if !isDirectChildPath(taskDir, cleaned) {
		return "", false
	}
	// The payload's parent must be exactly the task directory.
	if !pathIdentityEqual(filepath.Dir(cleaned), taskDir) {
		return "", false
	}
	return cleaned, true
}

// nvidiaDirectoryHoldsOnlyPayload reports whether taskDir contains exactly one
// entry, and that entry is the payload regular file. Any subdirectory, extra
// file, or reparse entry fails closed.
func nvidiaDirectoryHoldsOnlyPayload(deps nvidiaInstallerCacheDeps, taskDir, payloadPath string) bool {
	entries, err := deps.readDir(taskDir)
	if err != nil || len(entries) != 1 {
		return false
	}
	only := deps.joinPath(taskDir, entries[0].Name())
	if !pathIdentityEqual(only, payloadPath) {
		return false
	}
	info, err := deps.lstat(only)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 || !info.Mode().IsRegular() {
		return false
	}
	return true
}

// nvidiaChecksumMatches computes the payload's MD5 and compares it, case-
// insensitively, to the declared hex checksum. A malformed checksum or read
// failure fails closed.
func nvidiaChecksumMatches(deps nvidiaInstallerCacheDeps, payloadPath, checksum string) bool {
	checksum = strings.ToLower(strings.TrimSpace(checksum))
	if len(checksum) != md5.Size*2 {
		return false
	}
	if _, err := hex.DecodeString(checksum); err != nil {
		return false
	}
	file, err := deps.openFile(payloadPath)
	if err != nil {
		return false
	}
	defer file.Close()
	digest := md5.New()
	if _, err := io.Copy(digest, file); err != nil {
		return false
	}
	return hex.EncodeToString(digest.Sum(nil)) == checksum
}
