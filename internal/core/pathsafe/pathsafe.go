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

func ValidateDeletePath(path string) (Reason, bool) {
	return Validator{}.ValidateDeletePath(path)
}

func (v Validator) ValidateDeletePath(path string) (Reason, bool) {
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

	for _, protected := range []string{
		`c:\windows`,
		`c:\program files`,
		`c:\program files (x86)`,
	} {
		if cleaned == protected || strings.HasPrefix(cleaned, protected+`\`) {
			return reject("protected_path", "protected system paths cannot be cleaned by Foal")
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
