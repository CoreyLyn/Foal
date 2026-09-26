package clean

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CategoryVSCodeOutdatedExtensions is the opt-in category for extension
// versions that VS Code itself has marked for removal in its extensions
// folder's .obsolete file while another version of the same extension stays
// installed. VS Code deletes such folders during its own extension cleanup
// (deleteExtensionsMarkedForRemoval: when its shared process starts, when a
// remote server starts, or on "Developer: Cleanup Extensions Folder") without
// running uninstall hooks, because the extension remains installed. Fully
// uninstalled extensions are excluded: VS Code runs their vscode:uninstall
// hook and removes their global storage when it deletes them.
const CategoryVSCodeOutdatedExtensions = "vscode-outdated-extensions"

const vscodeOutdatedExtensionsOptInImpactNotice = "Opt-in outdated extension cleanup permanently deletes extension versions that VS Code has already marked for removal while another version of the same extension stays installed; VS Code would delete them itself during its next extension cleanup. The installed version, settings, and extension storage are kept. Permanent deletion is ordinary filesystem removal (not secure erasure)."

// Bounded metadata reads: extension metadata files are small JSON documents.
const (
	vscodeObsoleteMaxBytes       = 1 << 20
	vscodeExtensionsJSONMaxBytes = 16 << 20
	vscodeManifestMaxBytes       = 4 << 20
)

// vscodeExtensionFolders are the per-editor extension folders under the user
// profile. The OSS default ".vscode-oss" is excluded: VSCodium and source
// builds of Code - OSS share it, and Foal cannot attribute its processes.
var vscodeExtensionFolders = []string{".vscode", ".vscode-insiders"}

var vscodeOutdatedExtensionsPolicy = exactCandidatePolicy{
	resolveRoots: func(deps exactCandidateDeps) []exactCandidateRoot {
		profile, ok := deps.userProfile()
		if !ok {
			return nil
		}
		roots := make([]exactCandidateRoot, 0, len(vscodeExtensionFolders))
		for _, folder := range vscodeExtensionFolders {
			roots = append(roots, exactCandidateRoot{path: filepath.Join(profile, folder, "extensions")})
		}
		return roots
	},
	discover: discoverVSCodeOutdatedExtensions,
	// Both editors gate both folders: --extensions-dir can point either editor at
	// the other's folder.
	applications: []string{ApplicationVisualStudioCode, ApplicationVisualStudioCodeInsiders},
}

type vscodeExtensionsManifestEntry struct {
	Identifier struct {
		ID string `json:"id"`
	} `json:"identifier"`
	RelativeLocation string `json:"relativeLocation"`
}

type vscodeExtensionPackage struct {
	Publisher string `json:"publisher"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Metadata  struct {
		InstalledTimestamp *float64 `json:"installedTimestamp"`
		TargetPlatform     *string  `json:"targetPlatform"`
	} `json:"__metadata"`
}

// discoverVSCodeOutdatedExtensions returns extension folders that VS Code has
// marked for removal while the same extension stays installed. An extension
// stays installed only when the default profile's extensions.json references an
// existing, unmarked folder for it, so VS Code would never run its uninstall
// hook on the candidate's removal. It fails closed: missing or malformed
// .obsolete, extensions.json, or package.json, or any legacy manifest entry
// without relativeLocation, yields no candidate for that folder (or root).
func discoverVSCodeOutdatedExtensions(_ context.Context, _ exactCandidateDeps, root string) []string {
	var obsolete map[string]bool
	if !readBoundedJSON(filepath.Join(root, ".obsolete"), vscodeObsoleteMaxBytes, &obsolete) {
		return nil
	}
	marked := map[string]bool{}
	markedFolders := map[string]bool{}
	for key, value := range obsolete {
		if value {
			marked[key] = true
			markedFolders[strings.ToLower(key)] = true
		}
	}
	if len(marked) == 0 {
		return nil
	}
	var manifest []vscodeExtensionsManifestEntry
	if !readBoundedJSON(filepath.Join(root, "extensions.json"), vscodeExtensionsJSONMaxBytes, &manifest) {
		return nil
	}
	installed := map[string]bool{}
	referenced := map[string]bool{}
	for _, entry := range manifest {
		location := strings.TrimSpace(entry.RelativeLocation)
		if location == "" || location != filepath.Base(location) || location == "." || location == ".." {
			// Legacy or unexpected entries: VS Code migrates them itself.
			return nil
		}
		folder := strings.ToLower(location)
		referenced[folder] = true
		id := strings.ToLower(strings.TrimSpace(entry.Identifier.ID))
		if id == "" || markedFolders[folder] {
			continue
		}
		if folderID, ok := vscodeFolderExtensionID(filepath.Join(root, location)); ok && folderID == id {
			installed[id] = true
		}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var candidates []string
	for _, entry := range entries {
		name := entry.Name()
		if !markedFolders[strings.ToLower(name)] || referenced[strings.ToLower(name)] {
			continue
		}
		folder := filepath.Join(root, name)
		if !isRealDirectory(folder) {
			continue
		}
		var pkg vscodeExtensionPackage
		if !readBoundedJSON(filepath.Join(folder, "package.json"), vscodeManifestMaxBytes, &pkg) {
			continue
		}
		id, key, ok := vscodeExtensionKey(pkg)
		if !ok || !marked[key] || !strings.EqualFold(key, name) || !installed[id] {
			continue
		}
		candidates = append(candidates, folder)
	}
	sort.Slice(candidates, func(i, j int) bool { return strings.ToLower(candidates[i]) < strings.ToLower(candidates[j]) })
	return candidates
}

// vscodeExtensionKey mirrors VS Code's ExtensionKey.toString(): the lowercase
// extension id, "-", the version, and "-<targetPlatform>" unless the platform
// is absent or "undefined" (a "universal" platform keeps its suffix). Only
// extensions VS Code installed itself (installedTimestamp present) qualify; an
// empty platform string fails closed.
func vscodeExtensionKey(pkg vscodeExtensionPackage) (id, key string, ok bool) {
	publisher := strings.TrimSpace(pkg.Publisher)
	name := strings.TrimSpace(pkg.Name)
	version := strings.TrimSpace(pkg.Version)
	if publisher == "" || name == "" || version == "" || pkg.Metadata.InstalledTimestamp == nil || *pkg.Metadata.InstalledTimestamp <= 0 {
		return "", "", false
	}
	id = strings.ToLower(publisher + "." + name)
	key = id + "-" + version
	if platform := pkg.Metadata.TargetPlatform; platform != nil && *platform != "undefined" {
		if strings.TrimSpace(*platform) == "" {
			return "", "", false
		}
		key += "-" + *platform
	}
	return id, key, true
}

// vscodeFolderExtensionID returns the lowercase publisher.name id declared by
// the package.json of a real extension folder.
func vscodeFolderExtensionID(folder string) (string, bool) {
	if !isRealDirectory(folder) {
		return "", false
	}
	var pkg vscodeExtensionPackage
	if !readBoundedJSON(filepath.Join(folder, "package.json"), vscodeManifestMaxBytes, &pkg) {
		return "", false
	}
	publisher := strings.TrimSpace(pkg.Publisher)
	name := strings.TrimSpace(pkg.Name)
	if publisher == "" || name == "" {
		return "", false
	}
	return strings.ToLower(publisher + "." + name), true
}

// readBoundedJSON decodes one ordinary file of at most maxBytes into v.
func readBoundedJSON(path string, maxBytes int64, v any) bool {
	info, err := os.Lstat(path)
	if err != nil || !isOrdinaryFileInfo(info) || info.Size() > maxBytes {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes {
		return false
	}
	return json.Unmarshal(data, v) == nil
}
