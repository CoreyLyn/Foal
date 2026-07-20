//go:build windows

package uninstall

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// fakeRegistryKey satisfies registryKeyReader without touching the real
// Windows registry. A missing value returns registry.ErrNotExist exactly like
// registry.Key.GetStringValue / GetIntegerValue do.
type fakeRegistryKey struct {
	strings  map[string]string
	integers map[string]uint64
}

func (f fakeRegistryKey) GetStringValue(name string) (string, uint32, error) {
	if v, ok := f.strings[name]; ok {
		return v, registry.SZ, nil
	}
	return "", 0, registry.ErrNotExist
}

func (f fakeRegistryKey) GetIntegerValue(name string) (uint64, uint32, error) {
	if v, ok := f.integers[name]; ok {
		return v, registry.DWORD, nil
	}
	return 0, 0, registry.ErrNotExist
}

func TestReadApplicationEvidenceFromKeyExtractsUninstallCommandsAndInstallLocation(t *testing.T) {
	key := fakeRegistryKey{
		strings: map[string]string{
			"DisplayName":          "Example App",
			"DisplayVersion":       "1.2.3",
			"Publisher":            "Example Co",
			"QuietUninstallString": `MsiExec.exe /X{1234} /qn`,
			"UninstallString":      `MsiExec.exe /X{1234}`,
			"InstallLocation":      `C:\Program Files\Example App`,
		},
	}

	app, ok := readApplicationEvidenceFromKey(key, "windows_registry_uninstall_keys:HKLM64")
	if !ok {
		t.Fatalf("ok = false, want true for a displayable app")
	}
	if app.Name != "Example App" {
		t.Fatalf("name = %q, want Example App", app.Name)
	}
	if app.QuietUninstallCommand != `MsiExec.exe /X{1234} /qn` {
		t.Fatalf("quiet uninstall command = %q, want the QuietUninstallString value", app.QuietUninstallCommand)
	}
	if app.InteractiveUninstallCommand != `MsiExec.exe /X{1234}` {
		t.Fatalf("interactive uninstall command = %q, want the UninstallString value", app.InteractiveUninstallCommand)
	}
	if app.InstallLocation != `C:\Program Files\Example App` {
		t.Fatalf("install location = %q, want the InstallLocation value", app.InstallLocation)
	}
	if len(app.Sources) != 1 || app.Sources[0] != "windows_registry_uninstall_keys:HKLM64" {
		t.Fatalf("sources = %#v, want the passed source", app.Sources)
	}
}

func TestReadApplicationEvidenceFromKeySurfacesPartialUninstallEvidence(t *testing.T) {
	key := fakeRegistryKey{
		strings: map[string]string{
			"DisplayName":          "Quiet Only App",
			"QuietUninstallString": `MsiExec.exe /X{ABCD} /qn`,
		},
	}

	app, ok := readApplicationEvidenceFromKey(key, "windows_registry_uninstall_keys:HKCU")
	if !ok {
		t.Fatalf("ok = false, want true for an app with a DisplayName")
	}
	if app.QuietUninstallCommand == "" {
		t.Fatalf("quiet uninstall command = empty, want the QuietUninstallString value")
	}
	if app.InteractiveUninstallCommand != "" {
		t.Fatalf("interactive uninstall command = %q, want empty when UninstallString is absent", app.InteractiveUninstallCommand)
	}
	if app.InstallLocation != "" {
		t.Fatalf("install location = %q, want empty when InstallLocation is absent", app.InstallLocation)
	}
}

func TestReadApplicationEvidenceFromKeySkipsHiddenSystemComponent(t *testing.T) {
	key := fakeRegistryKey{
		integers: map[string]uint64{"SystemComponent": 1},
		strings:  map[string]string{"DisplayName": "Hidden App"},
	}

	if _, ok := readApplicationEvidenceFromKey(key, "windows_registry_uninstall_keys:HKLM64"); ok {
		t.Fatalf("ok = true, want false for SystemComponent=1 app")
	}
}

func TestReadApplicationEvidenceFromKeySkipsParentKeyChild(t *testing.T) {
	key := fakeRegistryKey{
		strings: map[string]string{
			"DisplayName":   "Child App",
			"ParentKeyName": "SomeParent",
		},
	}

	if _, ok := readApplicationEvidenceFromKey(key, "windows_registry_uninstall_keys:HKLM64"); ok {
		t.Fatalf("ok = true, want false for an app with ParentKeyName")
	}
}

func TestReadApplicationEvidenceFromKeySkipsEntryWithoutDisplayName(t *testing.T) {
	key := fakeRegistryKey{
		strings: map[string]string{
			"UninstallString": `C:\Uninstall.exe`,
		},
	}

	if _, ok := readApplicationEvidenceFromKey(key, "windows_registry_uninstall_keys:HKLM64"); ok {
		t.Fatalf("ok = true, want false for an app without DisplayName")
	}
}
