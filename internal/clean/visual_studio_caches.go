package clean

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// visualStudioSharedAllowlistedChildren are exact regenerable roots that sit
// directly under %LOCALAPPDATA%\Microsoft\VisualStudio (not under an instance
// hive). Order is part of deterministic discovery.
//
// Evidence: Microsoft Q&A documents deleting the shared Roslyn folder as a
// regenerable compiler/analyzer cache (rebuild cost only).
var visualStudioSharedAllowlistedChildren = []string{"Roslyn"}

// visualStudioInstanceAllowlistedChildren are exact regenerable roots under a
// matched Visual Studio instance directory. Order is part of deterministic
// discovery.
//
// Evidence: Microsoft documents ComponentModelCache (MEF component model cache)
// as safe to delete while Visual Studio is closed (devenv.exe not running); it
// rebuilds on next launch. Official Clear MEF Component Cache tooling targets
// the same root. Non-allowlisted siblings (Settings, Extensions, PackageCache,
// MEFCacheBackup, template caches, WebView2Cache, …) never become candidates.
var visualStudioInstanceAllowlistedChildren = []string{"ComponentModelCache"}

// visualStudioCachesOptInImpactNotice is path-free impact vocabulary for
// visual-studio-caches when Opt-in candidates are present.
const visualStudioCachesOptInImpactNotice = "Opt-in Visual Studio cache cleanup rebuilds MEF component caches and Roslyn analyzer caches. The next Visual Studio startup or solution load may be slower and may consume CPU/disk."

// visualStudioMinMajorMinor is the oldest instance major.minor still accepted
// (VS 2015 = 14.0). Pre-14 layouts and non-matching directory names fail closed.
const visualStudioMinMajor = 14

// resolveVisualStudioCacheRootScopes resolves the current user's standard
// %LOCALAPPDATA%\Microsoft\VisualStudio parent as a single product-scoped root
// gated by ApplicationVisualStudio (devenv.exe). Missing or blank Local AppData
// and a missing VisualStudio parent yield silent absence. Never reads install
// dirs, ProgramData, registry, Roaming settings, solutions, or vswhere.
func resolveVisualStudioCacheRootScopes(deps devCachePathDependencies) []DevCacheRootScope {
	localAppData, ok := deps.lookupEnv("LOCALAPPDATA")
	if !ok {
		return nil
	}
	localAppData = strings.TrimSpace(localAppData)
	if localAppData == "" {
		return nil
	}
	parent := deps.joinPath(localAppData, "Microsoft", "VisualStudio")
	if !isVisualStudioSafeDirectory(parent) {
		return nil
	}
	return []DevCacheRootScope{{
		Path:        parent,
		Application: ApplicationVisualStudio,
	}}
}

// discoverVisualStudioCacheChildren returns exact allowlisted regenerable roots
// under the VisualStudio parent. The parent itself is never a candidate.
// Unknown siblings, Settings, Extensions, and other non-allowlisted state are
// ignored. Fail closed when the root is missing or not a real directory.
func discoverVisualStudioCacheChildren(ctx context.Context, root string) []string {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	if !isVisualStudioSafeDirectory(root) {
		return nil
	}

	var children []string
	for _, name := range visualStudioSharedAllowlistedChildren {
		select {
		case <-ctx.Done():
			return children
		default:
		}
		child := filepath.Join(root, name)
		if isVisualStudioSafeDirectory(child) {
			children = append(children, child)
		}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return children
	}

	var instanceNames []string
	for _, entry := range entries {
		name := entry.Name()
		if !isVisualStudioInstanceDirName(name) {
			continue
		}
		instancePath := filepath.Join(root, name)
		if !isVisualStudioSafeDirectory(instancePath) {
			continue
		}
		instanceNames = append(instanceNames, name)
	}
	sort.SliceStable(instanceNames, func(i, j int) bool {
		return strings.ToLower(instanceNames[i]) < strings.ToLower(instanceNames[j])
	})

	for _, name := range instanceNames {
		for _, childName := range visualStudioInstanceAllowlistedChildren {
			select {
			case <-ctx.Done():
				return children
			default:
			}
			child := filepath.Join(root, name, childName)
			if isVisualStudioSafeDirectory(child) {
				children = append(children, child)
			}
		}
	}
	return children
}

// isVisualStudioInstanceDirName reports whether name is an anchored Visual
// Studio instance or version directory: major.minor or major.minor_<hex>, with
// major.minor at least 14.0 (VS 2015+). Decoys (BackupFiles, Packages, Roslyn,
// 13.0, trailing junk) fail closed.
func isVisualStudioInstanceDirName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	// Split optional instance id: "17.0_a4d9e95d" → version "17.0", id "a4d9e95d".
	versionPart := name
	if idx := strings.Index(name, "_"); idx >= 0 {
		versionPart = name[:idx]
		instanceID := name[idx+1:]
		if instanceID == "" || !isAllASCIIHex(instanceID) {
			return false
		}
	}
	return isVisualStudioSupportedVersion(versionPart)
}

// isVisualStudioSupportedVersion reports whether v is major.minor with major
// at least visualStudioMinMajor (14 = VS 2015).
func isVisualStudioSupportedVersion(v string) bool {
	if v == "" {
		return false
	}
	parts := strings.Split(v, ".")
	if len(parts) != 2 {
		return false
	}
	majorPart, minorPart := parts[0], parts[1]
	if majorPart == "" || minorPart == "" {
		return false
	}
	if !isAllASCIIDigits(majorPart) || !isAllASCIIDigits(minorPart) {
		return false
	}
	// Disallow leading zeros on major (014.0) and multi-digit minor (17.01).
	if (len(majorPart) > 1 && majorPart[0] == '0') || (len(minorPart) > 1 && minorPart[0] == '0') {
		return false
	}
	major, err := strconv.Atoi(majorPart)
	if err != nil {
		return false
	}
	if _, err := strconv.Atoi(minorPart); err != nil {
		return false
	}
	return major >= visualStudioMinMajor
}

// isAllASCIIHex reports whether s is non-empty and only 0-9 / a-f / A-F.
func isAllASCIIHex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
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

// isVisualStudioSafeDirectory reports whether path exists as a real directory
// (not a regular file, symlink, junction, or other reparse point).
func isVisualStudioSafeDirectory(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return info.IsDir()
}
