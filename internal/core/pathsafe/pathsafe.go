package pathsafe

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

type Reason struct {
	Code    string
	Message string
}

type Validator struct {
	userProtectionRules []protectionRule
}

type protectionRule struct {
	path       string
	normalized string
}

func NewValidator(userProtectionPaths []string) Validator {
	rules := make([]protectionRule, 0, len(userProtectionPaths))
	seen := make(map[string]struct{}, len(userProtectionPaths))
	for _, path := range userProtectionPaths {
		displayPath, normalized, _, ok := NormalizeProtectionPath(path)
		if !ok {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		rules = append(rules, protectionRule{
			path:       displayPath,
			normalized: normalized,
		})
	}
	sort.SliceStable(rules, func(i, j int) bool {
		return rules[i].normalized < rules[j].normalized
	})
	effective := rules[:0]
	for _, rule := range rules {
		covered := false
		for _, ancestor := range effective {
			if isSameOrDescendant(rule.normalized, ancestor.normalized) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		effective = append(effective, rule)
	}
	return Validator{userProtectionRules: effective}
}

func NormalizeProtectionPath(path string) (string, string, Reason, bool) {
	displayPath, normalized := normalizeLocalPath(path)
	if normalized == "" {
		reason, ok := reject("empty_path", "Protection path cannot be empty")
		return "", "", reason, ok
	}
	if strings.HasPrefix(normalized, `\\`) {
		reason, ok := reject("unc_path", "UNC paths cannot be used as Protection rules")
		return "", "", reason, ok
	}
	if !filepath.IsAbs(normalized) {
		reason, ok := reject("relative_path", "Protection path must be absolute")
		return "", "", reason, ok
	}
	if containsShortNameSegment(normalized) {
		reason, ok := reject("short_name_path", "8.3 short-name paths cannot be used as Protection rules")
		return "", "", reason, ok
	}
	return displayPath, normalized, Reason{}, true
}

func (v Validator) UserProtectionPaths() []string {
	paths := make([]string, 0, len(v.userProtectionRules))
	for _, rule := range v.userProtectionRules {
		paths = append(paths, rule.path)
	}
	return paths
}

func (v Validator) IsUserProtected(path string) bool {
	_, normalized, _, ok := NormalizeProtectionPath(path)
	if !ok {
		return false
	}
	for _, protected := range v.userProtectionRules {
		if isSameOrDescendant(normalized, protected.normalized) {
			return true
		}
	}
	return false
}

// Built-in system trees that must never be cleaned or used as deep-scan roots.
var builtInProtectedSystemRoots = []string{
	`c:\windows`,
	`c:\program files`,
	`c:\program files (x86)`,
}

func ValidateDeletePath(path string) (Reason, bool) {
	return Validator{}.ValidateDeletePath(path)
}

// ValidateUserScanRoot reports whether path is acceptable as an explicit
// user-supplied deep-scan root (purge). It rejects empty paths, UNC, relative
// forms, 8.3 short names, volume roots (e.g. C:\), well-known system trees
// (Windows, Program Files), and the current user's profile directory without
// reading the candidate filesystem tree. Callers must still verify the path
// exists, is a directory, and is not a reparse point.
//
// Project paths under the profile (e.g. %USERPROFILE%\Projects\foo) remain
// allowed; only the profile root itself is rejected as too broad.
//
// A failed reason uses code "dangerous_root" for policy rejections so purge can
// fail closed before any partial scan of system trees.
func ValidateUserScanRoot(path string) (Reason, bool) {
	if strings.TrimSpace(path) == "" {
		return reject("empty_path", "scan root cannot be empty")
	}

	normalized := stripLongPathPrefix(path)
	cleaned := strings.ToLower(filepath.Clean(normalized))

	if strings.HasPrefix(cleaned, `\\`) {
		return reject("unc_path", "UNC paths cannot be used as purge roots")
	}
	if !filepath.IsAbs(cleaned) {
		return reject("relative_path", "scan root must be absolute")
	}
	if containsShortNameSegment(cleaned) {
		return reject("short_name_path", "8.3 short-name paths cannot be used as purge roots")
	}
	if isVolumeRootPath(cleaned) {
		return reject("dangerous_root", "volume roots cannot be used as purge roots; supply a project or workspace directory")
	}
	for _, protected := range builtInProtectedSystemRoots {
		if cleaned == protected || strings.HasPrefix(cleaned, protected+string(filepath.Separator)) {
			return reject("dangerous_root", "system paths such as Windows and Program Files cannot be used as purge roots")
		}
	}
	if isCurrentUserProfileRoot(cleaned) {
		return reject("dangerous_root", "user profile roots cannot be used as purge roots; supply a project or workspace directory")
	}
	return Reason{}, true
}

// isCurrentUserProfileRoot reports whether cleaned (filepath.Clean + lower) is
// exactly the current user's home / USERPROFILE directory. Descendants are not
// matched so ordinary project paths under the profile stay valid.
func isCurrentUserProfileRoot(cleaned string) bool {
	for _, candidate := range currentUserProfileRoots() {
		if candidate != "" && cleaned == candidate {
			return true
		}
	}
	return false
}

func currentUserProfileRoots() []string {
	var roots []string
	seen := make(map[string]struct{}, 2)
	add := func(raw string) {
		raw = strings.TrimSpace(stripLongPathPrefix(raw))
		if raw == "" {
			return
		}
		normalized := strings.ToLower(filepath.Clean(raw))
		if normalized == "" || !filepath.IsAbs(normalized) {
			return
		}
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		roots = append(roots, normalized)
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(home)
	}
	add(os.Getenv("USERPROFILE"))
	return roots
}

// isVolumeRootPath reports whether cleaned (filepath.Clean + lower) is a drive
// volume root such as c:\ or c:.
func isVolumeRootPath(cleaned string) bool {
	vol := strings.ToLower(filepath.VolumeName(cleaned))
	if vol == "" {
		return false
	}
	rest := strings.TrimPrefix(cleaned, vol)
	rest = strings.Trim(rest, `/\`)
	return rest == ""
}

func (v Validator) ValidateDeletePath(path string) (Reason, bool) {
	return v.validateMutationPath(path, false /* portableMode */)
}

// ValidatePortableRemovalPath reports whether path is acceptable as a portable
// install location to be permanently removed by Uninstall execution. It is the
// one Foal mutation path that may legitimately target a per-app directory under
// Program Files or AppData (e.g. C:\Program Files\MyApp), because those are
// plausible portable install roots. The built-in protected system roots check
// in ValidateDeletePath would reject all Program Files descendants, which is
// correct for Clean/Purge but wrong for portable removal.
//
// The portable root policy still rejects volume roots (C:\), the Windows system
// tree (root and descendants), the Program Files roots themselves (exact only,
// descendants allowed), the current user's profile root (exact only), and the
// AppData roots themselves (exact only, descendants allowed). User-defined
// Protection rules apply deny-only as on every other mutation path. Reparse
// points and multi-hardlink files are rejected. Callers must still verify the
// path is a directory before removal.
//
// This mirrors the "trusted install location" policy from the Uninstall
// execution spec: be conservative, reject dangerous roots, and when in doubt
// skip with a stable reason rather than permanently deleting a dangerous root.
func (v Validator) ValidatePortableRemovalPath(path string) (Reason, bool) {
	return v.validateMutationPath(path, true /* portableMode */)
}

// validateMutationPath is the shared validation core for ValidateDeletePath and
// ValidatePortableRemovalPath. The two differ only in the system-root policy:
// the standard mode rejects all descendants of built-in protected system roots
// (Windows, Program Files, Program Files (x86)) as protected_path, while
// portable mode applies a portable-specific dangerous_root policy that rejects
// roots and genuinely dangerous trees but allows per-app descendants under
// Program Files and AppData. All other checks (format, user Protection rules,
// Lstat, reparse, hardlink) are identical.
func (v Validator) validateMutationPath(path string, portableMode bool) (Reason, bool) {
	if strings.TrimSpace(path) == "" {
		return reject("empty_path", "delete path cannot be empty")
	}

	normalized := stripLongPathPrefix(path)
	cleaned := strings.ToLower(filepath.Clean(normalized))

	if strings.HasPrefix(cleaned, `\\`) {
		return reject("unc_path", "UNC paths require explicit handling and cannot be cleaned by default")
	}
	if !filepath.IsAbs(cleaned) {
		return reject("relative_path", "delete path must be absolute")
	}
	if containsShortNameSegment(cleaned) {
		return reject("short_name_path", "8.3 short-name paths cannot be used for cleanup safety decisions")
	}

	for _, protected := range v.userProtectionRules {
		if isSameOrDescendant(cleaned, protected.normalized) {
			return reject("protected_path", "path is protected by user-defined Protection rule: "+protected.path)
		}
	}

	if portableMode {
		if reason, ok := validatePortableRootPolicy(cleaned); !ok {
			return reason, false
		}
	} else {
		for _, protected := range builtInProtectedSystemRoots {
			if cleaned == protected || strings.HasPrefix(cleaned, protected+`\`) {
				return reject("protected_path", "protected system paths cannot be cleaned by Foal")
			}
		}
	}

	info, err := os.Lstat(normalized)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return reject("permission_denied", "permission denied while validating delete path")
		}
		return reject("stat_failed", "delete path could not be inspected before cleanup")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return reject("reparse_point", "reparse points cannot be cleaned by default")
	}

	reparse, err := hasReparseAttribute(normalized)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return reject("permission_denied", "permission denied while reading Windows file attributes")
		}
		return reject("stat_failed", "Windows file attributes could not be inspected before cleanup")
	}
	if reparse {
		return reject("reparse_point", "reparse points cannot be cleaned by default")
	}

	hardlink, err := hasMultipleHardlinks(normalized)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return reject("permission_denied", "permission denied while checking hardlink count")
		}
		return reject("stat_failed", "hardlink count could not be inspected before cleanup")
	}
	if hardlink {
		return reject("hardlink_path", "files with multiple hardlinks cannot be cleaned by default")
	}

	return Reason{}, true
}

// validatePortableRootPolicy is the portable-removal-specific system root
// policy. It rejects volume roots, the Windows system tree (root and
// descendants), the Program Files roots themselves (exact only; per-app
// descendants remain eligible), the current user's profile root (exact only),
// and the AppData roots themselves (exact only; per-app descendants remain
// eligible). It returns a dangerous_root reason so callers record a stable
// skip code consistent with Purge's dangerous_root vocabulary.
func validatePortableRootPolicy(cleaned string) (Reason, bool) {
	if isVolumeRootPath(cleaned) {
		return reject("dangerous_root", "volume roots cannot be permanently removed as portable install locations")
	}
	// The Windows system tree is always dangerous: reject root and descendants.
	if isSameOrDescendant(cleaned, `c:\windows`) {
		return reject("dangerous_root", "the Windows system tree cannot be permanently removed")
	}
	// Program Files roots are rejected as exact roots only; per-app descendants
	// (e.g. C:\Program Files\MyApp) are plausible portable install locations.
	if cleaned == `c:\program files` || cleaned == `c:\program files (x86)` {
		return reject("dangerous_root", "Program Files root cannot be permanently removed; only per-app install directories are eligible")
	}
	// User profile root: reject the root itself, allow descendants.
	if isCurrentUserProfileRoot(cleaned) {
		return reject("dangerous_root", "user profile root cannot be permanently removed")
	}
	// AppData roots: reject the roots themselves, allow per-app descendants.
	for _, root := range currentUserAppDataRoots() {
		if cleaned == root {
			return reject("dangerous_root", "AppData root cannot be permanently removed; only per-app install directories are eligible")
		}
	}
	return Reason{}, true
}

// currentUserAppDataRoots returns the current user's standard AppData roots
// (LOCALAPPDATA and APPDATA), normalized for comparison. Used by the portable
// removal root policy to reject the AppData roots themselves while allowing
// per-app descendants. Returns an empty slice when the environment variables
// are unset (e.g. on non-Windows or service accounts), in which case no AppData
// root match is possible and descendants are evaluated by the remaining checks.
func currentUserAppDataRoots() []string {
	var roots []string
	seen := make(map[string]struct{}, 2)
	add := func(raw string) {
		raw = strings.TrimSpace(stripLongPathPrefix(raw))
		if raw == "" {
			return
		}
		normalized := strings.ToLower(filepath.Clean(raw))
		if normalized == "" || !filepath.IsAbs(normalized) {
			return
		}
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		roots = append(roots, normalized)
	}
	add(os.Getenv("LOCALAPPDATA"))
	add(os.Getenv("APPDATA"))
	return roots
}

func normalizeLocalPath(path string) (string, string) {
	path = strings.TrimSpace(stripLongPathPrefix(path))
	if path == "" {
		return "", ""
	}
	cleaned := filepath.Clean(path)
	return cleaned, strings.ToLower(cleaned)
}

func isSameOrDescendant(path, root string) bool {
	if path == root {
		return true
	}
	if strings.HasSuffix(root, string(filepath.Separator)) {
		return strings.HasPrefix(path, root)
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

func reject(code, message string) (Reason, bool) {
	return Reason{Code: code, Message: message}, false
}

func stripLongPathPrefix(path string) string {
	if strings.HasPrefix(path, `\\?\UNC\`) {
		return `\\` + strings.TrimPrefix(path, `\\?\UNC\`)
	}
	return strings.TrimPrefix(path, `\\?\`)
}

func containsShortNameSegment(path string) bool {
	for _, segment := range strings.FieldsFunc(path, func(r rune) bool {
		return r == '\\' || r == '/'
	}) {
		if strings.Contains(segment, "~") {
			return true
		}
	}
	return false
}

func hasReparseAttribute(path string) (bool, error) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	attrs, err := syscall.GetFileAttributes(ptr)
	if err != nil {
		return false, err
	}
	return attrs&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}

func hasMultipleHardlinks(path string) (bool, error) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	handle, err := syscall.CreateFile(
		ptr,
		0,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return false, err
	}
	defer syscall.CloseHandle(handle)

	var data syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(handle, &data); err != nil {
		return false, err
	}
	if data.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return false, nil
	}
	return data.NumberOfLinks > 1, nil
}

// NormalizePathForIdentity normalizes a path for identity comparison purposes.
// It handles:
// - Case insensitivity (lowercase)
// - Stripping Windows long-path prefixes
// - Cleaning redundant separators
// - Removing trailing separators
// Empty or whitespace-only paths return empty string.
func NormalizePathForIdentity(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = stripLongPathPrefix(path)
	path = filepath.Clean(path)
	return strings.ToLower(path)
}

// PathsAreSameIdentity reports whether two paths represent the same logical
// path on Windows, considering case insensitivity, separators, and long-path
// prefixes.
func PathsAreSameIdentity(a, b string) bool {
	return NormalizePathForIdentity(a) == NormalizePathForIdentity(b)
}

// IsEmptyOrWhitespacePath reports whether the path is empty or only whitespace.
func IsEmptyOrWhitespacePath(path string) bool {
	return strings.TrimSpace(path) == ""
}
