package clean

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// jetbrainsIDEProductPolicy is one private IntelliJ-platform product entry for
// jetbrains-ide-caches. Prefixes, launcher executables, and optional extra cache
// children stay private; the public category interface is only the canonical id.
//
// Directory matching is anchored: a direct child of %LOCALAPPDATA%\JetBrains must
// be exactly <prefix><YYYY.N> with YYYY.N >= 2020.1. Substring decoys and
// pre-2020 layouts fail closed.
type jetbrainsIDEProductPolicy struct {
	// application is the logical application identity for product-scoped
	// idle-before-and-after gating. Multiple edition prefixes share one identity.
	application string
	// prefixes are system-directory product prefixes (ASCII). Longer prefixes must
	// be listed before shorter ones that are prefixes of them (e.g. PyCharmCE
	// before PyCharm) so matching is unambiguous.
	prefixes []string
	// extraCacheChildren are product-specific allowlisted children beyond the
	// shared caches/index list (empty for IDEA/PyCharm; Rider uses resharper-host).
	extraCacheChildren []string
}

// jetbrainsIDEProductPolicies is the fail-closed product catalog for this slice.
// #208 ships IntelliJ IDEA Ultimate/Community and PyCharm Professional/Community.
// Additional JetBrains IDEs require a deliberate catalog + test expansion.
var jetbrainsIDEProductPolicies = []jetbrainsIDEProductPolicy{
	{
		application: ApplicationIntelliJIDEA,
		// IntelliJIdea = Ultimate; IdeaIC = Community Edition.
		prefixes: []string{"IntelliJIdea", "IdeaIC"},
	},
	{
		application: ApplicationPyCharm,
		// PyCharmCE before PyCharm so Community is not absorbed by Professional.
		prefixes: []string{"PyCharmCE", "PyCharm"},
	},
}

// jetbrainsSharedCacheChildren is the common exact allowlist under every
// supported product-version system root. Order is part of deterministic discovery.
var jetbrainsSharedCacheChildren = []string{"caches", "index"}

// jetbrainsIDECachesOptInImpactNotice is path-free impact vocabulary for
// jetbrains-ide-caches when Opt-in candidates are present.
const jetbrainsIDECachesOptInImpactNotice = "Opt-in JetBrains IDE cache cleanup rebuilds indexes. The next IDE startup or project open may be slower and may consume CPU/disk."

// resolveJetBrainsIDECacheRootScopes resolves product-version system roots under
// the current user's standard %LOCALAPPDATA%\JetBrains directory. Each root is a
// product-scoped DevCacheRootScope (Application set). Missing or blank Local
// AppData, a missing JetBrains parent, and non-matching children yield no scopes.
// Never reads config roots, Toolbox, registry, CWD, projects, or idea.system.path.
func resolveJetBrainsIDECacheRootScopes(deps devCachePathDependencies) []DevCacheRootScope {
	localAppData, ok := deps.lookupEnv("LOCALAPPDATA")
	if !ok {
		return nil
	}
	localAppData = strings.TrimSpace(localAppData)
	if localAppData == "" {
		return nil
	}
	parent := deps.joinPath(localAppData, "JetBrains")
	if !isJetBrainsSafeDirectory(parent) {
		return nil
	}

	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil
	}

	type matchedRoot struct {
		path         string
		application  string
		productIndex int
		version      string
		name         string
	}
	var matched []matchedRoot
	for _, entry := range entries {
		name := entry.Name()
		policy, productIndex, version, ok := matchJetBrainsProductVersionDir(name)
		if !ok {
			continue
		}
		path := deps.joinPath(parent, name)
		if !isJetBrainsSafeDirectory(path) {
			continue
		}
		matched = append(matched, matchedRoot{
			path:         path,
			application:  policy.application,
			productIndex: productIndex,
			version:      version,
			name:         name,
		})
	}

	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].productIndex != matched[j].productIndex {
			return matched[i].productIndex < matched[j].productIndex
		}
		if cmp := compareJetBrainsVersions(matched[i].version, matched[j].version); cmp != 0 {
			return cmp < 0
		}
		return strings.ToLower(matched[i].name) < strings.ToLower(matched[j].name)
	})

	scopes := make([]DevCacheRootScope, 0, len(matched))
	for _, m := range matched {
		scopes = append(scopes, DevCacheRootScope{
			Path:        m.path,
			Application: m.application,
		})
	}
	return scopes
}

// discoverJetBrainsIDECacheChildren returns exact allowlisted cache children
// under one product-version system root. The product root itself is never
// returned. Unknown children, Local History, plugins, logs, and other excluded
// state are ignored. Fail closed when the root name is not a supported product.
func discoverJetBrainsIDECacheChildren(ctx context.Context, root string) []string {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	base := filepath.Base(root)
	policy, _, _, ok := matchJetBrainsProductVersionDir(base)
	if !ok {
		return nil
	}
	if !isJetBrainsSafeDirectory(root) {
		return nil
	}

	childNames := make([]string, 0, len(jetbrainsSharedCacheChildren)+len(policy.extraCacheChildren))
	childNames = append(childNames, jetbrainsSharedCacheChildren...)
	childNames = append(childNames, policy.extraCacheChildren...)

	var children []string
	for _, name := range childNames {
		select {
		case <-ctx.Done():
			return children
		default:
		}
		child := filepath.Join(root, name)
		if !isJetBrainsSafeDirectory(child) {
			continue
		}
		children = append(children, child)
	}
	return children
}

// matchJetBrainsProductVersionDir reports whether name is an anchored
// product-version system directory for a catalogued product. productIndex is
// the stable catalog ordinal used for deterministic ordering.
func matchJetBrainsProductVersionDir(name string) (policy jetbrainsIDEProductPolicy, productIndex int, version string, ok bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return jetbrainsIDEProductPolicy{}, 0, "", false
	}
	lower := strings.ToLower(name)

	type candidate struct {
		policy       jetbrainsIDEProductPolicy
		productIndex int
		prefixLen    int
	}
	var best *candidate
	for i, p := range jetbrainsIDEProductPolicies {
		for _, prefix := range p.prefixes {
			pl := strings.ToLower(prefix)
			if !strings.HasPrefix(lower, pl) {
				continue
			}
			// Anchored: prefix must be followed by a version, not more product text
			// that is handled by a longer catalog prefix.
			if best != nil && len(pl) <= best.prefixLen {
				continue
			}
			best = &candidate{policy: p, productIndex: i, prefixLen: len(pl)}
		}
	}
	if best == nil {
		return jetbrainsIDEProductPolicy{}, 0, "", false
	}
	versionPart := name[best.prefixLen:]
	if !isJetBrainsSupportedVersion(versionPart) {
		return jetbrainsIDEProductPolicy{}, 0, "", false
	}
	return best.policy, best.productIndex, versionPart, true
}

// isJetBrainsSupportedVersion reports whether v is a 2020.1+ major.minor token
// (digits.digits) used by JetBrains standard Windows system directories.
func isJetBrainsSupportedVersion(v string) bool {
	if v == "" {
		return false
	}
	// Reject trailing junk (EAP suffixes, extra segments, spaces).
	parts := strings.Split(v, ".")
	if len(parts) != 2 {
		return false
	}
	yearPart, minorPart := parts[0], parts[1]
	if yearPart == "" || minorPart == "" {
		return false
	}
	if !isAllASCIIDigits(yearPart) || !isAllASCIIDigits(minorPart) {
		return false
	}
	// Disallow leading zeros (2020.01 / 02020.1 are not standard layouts).
	if len(yearPart) != 4 || (len(minorPart) > 1 && minorPart[0] == '0') {
		return false
	}
	year, err := strconv.Atoi(yearPart)
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(minorPart)
	if err != nil {
		return false
	}
	if year < 2020 {
		return false
	}
	if year == 2020 && minor < 1 {
		return false
	}
	return minor >= 1
}

// compareJetBrainsVersions returns -1/0/1 for ascending major.minor order.
// Invalid versions sort as empty (callers only pass validated versions).
func compareJetBrainsVersions(a, b string) int {
	ay, am, aok := parseJetBrainsVersionParts(a)
	by, bm, bok := parseJetBrainsVersionParts(b)
	if !aok && !bok {
		return 0
	}
	if !aok {
		return -1
	}
	if !bok {
		return 1
	}
	if ay != by {
		if ay < by {
			return -1
		}
		return 1
	}
	if am != bm {
		if am < bm {
			return -1
		}
		return 1
	}
	return 0
}

func parseJetBrainsVersionParts(v string) (year, minor int, ok bool) {
	if !isJetBrainsSupportedVersion(v) {
		return 0, 0, false
	}
	parts := strings.Split(v, ".")
	year, _ = strconv.Atoi(parts[0])
	minor, _ = strconv.Atoi(parts[1])
	return year, minor, true
}

// isJetBrainsSafeDirectory reports whether path exists as a real directory
// (not a regular file, symlink, junction, or other reparse point).
func isJetBrainsSafeDirectory(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return info.IsDir()
}
