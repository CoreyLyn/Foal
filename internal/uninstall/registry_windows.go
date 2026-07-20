//go:build windows

package uninstall

import (
	"errors"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const registryUninstallPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`

type registryUninstallRoot struct {
	key    registry.Key
	path   string
	access uint32
	source string
}

// registryKeyReader is the subset of registry.Key used to read a single
// uninstall entry's values. Defining it as an interface keeps entry parsing
// testable without opening a real Windows registry key.
type registryKeyReader interface {
	GetStringValue(name string) (string, uint32, error)
	GetIntegerValue(name string) (uint64, uint32, error)
}

func discoverPlatformUninstallEvidence() DiscoveryResult {
	roots := []registryUninstallRoot{
		{key: registry.LOCAL_MACHINE, path: registryUninstallPath, access: registry.READ | registry.WOW64_64KEY, source: "windows_registry_uninstall_keys:HKLM64"},
		{key: registry.LOCAL_MACHINE, path: registryUninstallPath, access: registry.READ | registry.WOW64_32KEY, source: "windows_registry_uninstall_keys:HKLM32"},
		{key: registry.CURRENT_USER, path: registryUninstallPath, access: registry.READ, source: "windows_registry_uninstall_keys:HKCU"},
	}

	result := DiscoveryResult{}
	for _, root := range roots {
		discoverRegistryRoot(root, &result)
	}
	return result
}

func discoverRegistryRoot(root registryUninstallRoot, result *DiscoveryResult) {
	key, err := registry.OpenKey(root.key, root.path, root.access)
	if err != nil {
		result.Sources = append(result.Sources, EvidenceSource{Source: root.source, Status: "skipped", Reason: err.Error()})
		result.Skipped = append(result.Skipped, SkippedReason{Source: root.source, Reason: registryDiscoveryFailureReason(err), Recoverable: true})
		return
	}
	defer key.Close()

	names, err := key.ReadSubKeyNames(-1)
	if err != nil {
		result.Sources = append(result.Sources, EvidenceSource{Source: root.source, Status: "skipped", Reason: err.Error()})
		result.Skipped = append(result.Skipped, SkippedReason{Source: root.source, Reason: "registry_discovery_failed", Recoverable: true})
		return
	}

	reported := false
	for _, name := range names {
		app, ok, err := readRegistryApplication(key, name, root.access, root.source)
		if err != nil {
			entrySource := root.source + `\` + name
			result.Skipped = append(result.Skipped, SkippedReason{Source: entrySource, Reason: "registry_entry_read_failed", Recoverable: true})
			continue
		}
		if !ok {
			continue
		}
		reported = true
		result.Evidence.Applications = append(result.Evidence.Applications, app)
	}

	if reported {
		result.Sources = append(result.Sources, EvidenceSource{Source: root.source, Status: "reported"})
		return
	}
	result.Sources = append(result.Sources, EvidenceSource{Source: root.source, Status: "reported", Reason: "no displayable applications found"})
}

func readRegistryApplication(parent registry.Key, subkeyName string, access uint32, source string) (ApplicationEvidence, bool, error) {
	key, err := registry.OpenKey(parent, subkeyName, access)
	if err != nil {
		return ApplicationEvidence{}, false, err
	}
	defer key.Close()

	app, ok := readApplicationEvidenceFromKey(key, source)
	return app, ok, nil
}

// readApplicationEvidenceFromKey parses one registry uninstall entry from an
// already-open key. It returns ok=false for hidden system components, entries
// with a parent key, or entries without a DisplayName; otherwise it surfaces
// the quiet and interactive uninstall commands and install location so the
// review layer can classify the app's planned execution class.
func readApplicationEvidenceFromKey(key registryKeyReader, source string) (ApplicationEvidence, bool) {
	if hiddenRegistryApplication(key) {
		return ApplicationEvidence{}, false
	}
	name := registryStringValue(key, "DisplayName")
	if name == "" {
		return ApplicationEvidence{}, false
	}

	return ApplicationEvidence{
		Name:                        name,
		Version:                     registryStringValue(key, "DisplayVersion"),
		Publisher:                   registryStringValue(key, "Publisher"),
		QuietUninstallCommand:       registryStringValue(key, "QuietUninstallString"),
		InteractiveUninstallCommand: registryStringValue(key, "UninstallString"),
		InstallLocation:             registryStringValue(key, "InstallLocation"),
		Sources:                     []string{source},
	}, true
}

func hiddenRegistryApplication(key registryKeyReader) bool {
	systemComponent, _, err := key.GetIntegerValue("SystemComponent")
	if err == nil && systemComponent == 1 {
		return true
	}
	return registryStringValue(key, "ParentKeyName") != ""
}

func registryStringValue(key registryKeyReader, name string) string {
	value, _, err := key.GetStringValue(name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func registryDiscoveryFailureReason(err error) string {
	if errors.Is(err, registry.ErrNotExist) {
		return "registry_source_unavailable"
	}
	return "registry_discovery_failed"
}
