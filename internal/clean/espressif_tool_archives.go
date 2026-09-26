package clean

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CategoryEspressifToolArchives is the opt-in category for ESP-IDF tool
// archives in the documented download folder ("dist": where idf_tools.py
// downloads tool archives; tools are extracted into the separate "tools"
// folder and run from there). Roots: IDF_TOOLS_PATH\dist and
// %USERPROFILE%\.espressif\dist when that tools directory carries an ESP-IDF
// marker (a "tools" folder or idf-env.json), and the ESP-IDF Installation
// Manager default %SystemDrive%\Espressif\dist only when its
// tools\eim_idf.json installation record exists (EIM hardcodes C:\Espressif, so
// a non-C: system drive yields no root). Only direct ordinary archive files in
// idf_tools.py's unpack formats that have been quiet for 24 hours are
// candidates; partial ".tmp" downloads never match.
const CategoryEspressifToolArchives = "espressif-tool-archives"

// ApplicationESPIDFInstallationManager is the ESP-IDF Installation Manager
// (EIM); its GUI and CLI are one executable.
const ApplicationESPIDFInstallationManager = "esp-idf-installation-manager"

const espressifToolArchivesOptInImpactNotice = "Opt-in ESP-IDF tool archive cleanup permanently deletes downloaded tool archives from the ESP-IDF download folder, including archives of the installed tool versions and archives staged for a later offline install. Installed tools keep working; repairing, reinstalling, or installing tools again downloads the archives, so offline installs need network access. Permanent deletion is ordinary filesystem removal (not secure erasure)."

// espressifArchiveQuietPeriod keeps recently written archives — an install may
// still be downloading or extracting them — out of the candidate set.
const espressifArchiveQuietPeriod = 24 * time.Hour

// espressifArchiveSuffixes are the archive formats idf_tools.py unpacks.
var espressifArchiveSuffixes = []string{".zip", ".tar.gz", ".tar.xz", ".tar.bz2", ".tgz"}

var espressifToolArchivesPolicy = exactCandidatePolicy{
	resolveRoots: resolveEspressifDistRoots,
	discover:     discoverEspressifToolArchives,
	applications: []string{ApplicationESPIDFInstallationManager},
}

func resolveEspressifDistRoots(deps exactCandidateDeps) []exactCandidateRoot {
	var roots []exactCandidateRoot
	var toolsDirs []string
	if toolsPath, ok := deps.envPath("IDF_TOOLS_PATH"); ok {
		toolsDirs = append(toolsDirs, toolsPath)
	}
	if profile, ok := deps.userProfile(); ok {
		toolsDirs = append(toolsDirs, filepath.Join(profile, ".espressif"))
	}
	for _, toolsDir := range toolsDirs {
		if isRealDirectory(filepath.Join(toolsDir, "tools")) || isOrdinaryFile(filepath.Join(toolsDir, "idf-env.json")) {
			roots = append(roots, exactCandidateRoot{path: filepath.Join(toolsDir, "dist")})
		}
	}
	if systemDrive, ok := deps.lookupEnv("SystemDrive"); ok && strings.TrimSpace(systemDrive) != "" {
		base := filepath.Join(strings.TrimSpace(systemDrive)+string(filepath.Separator), "Espressif")
		if isOrdinaryFile(filepath.Join(base, "tools", "eim_idf.json")) {
			roots = append(roots, exactCandidateRoot{path: filepath.Join(base, "dist")})
		}
	}
	return roots
}

// isOrdinaryFile reports whether path is an existing regular, non-reparse file.
func isOrdinaryFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && isOrdinaryFileInfo(info)
}

func isEspressifArchiveName(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range espressifArchiveSuffixes {
		if strings.HasSuffix(lower, suffix) && len(lower) > len(suffix) {
			return true
		}
	}
	return false
}

func discoverEspressifToolArchives(_ context.Context, deps exactCandidateDeps, root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	now := deps.now()
	var archives []string
	for _, entry := range entries {
		if !isEspressifArchiveName(entry.Name()) {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !isOrdinaryFileInfo(info) {
			continue
		}
		modified := info.ModTime()
		if modified.IsZero() || modified.After(now) || now.Sub(modified) < espressifArchiveQuietPeriod {
			continue
		}
		archives = append(archives, path)
	}
	sort.Slice(archives, func(i, j int) bool { return strings.ToLower(archives[i]) < strings.ToLower(archives[j]) })
	return archives
}
