package clean_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

func TestLoadProtectionConfigurationKeepsValidEntriesAndDiagnosesInvalidLines(t *testing.T) {
	appData := t.TempDir()
	configDir := filepath.Join(appData, "Foal")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "protection.txt")
	if err := os.WriteFile(configPath, []byte("\n  # valuable work\n  C:\\Work\\重要资料  \nrelative\\cache\n\\\\server\\share\nC:\\PROGRA~1\\cache\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPDATA", appData)
	t.Setenv("FOAL_PROTECTION_FILE", "")

	config := clean.LoadProtectionConfiguration()

	if config.LoadError != nil {
		t.Fatalf("LoadError = %#v, want nil", config.LoadError)
	}
	paths := config.Validator.UserProtectionPaths()
	if len(paths) != 1 || paths[0] != `C:\Work\重要资料` {
		t.Fatalf("active paths = %#v, want one trimmed UTF-8 path", paths)
	}
	if len(config.Diagnostics) != 3 {
		t.Fatalf("diagnostics = %#v, want three invalid-line diagnostics", config.Diagnostics)
	}
	for index, want := range []struct {
		code string
		line int
	}{
		{code: "relative_path", line: 4},
		{code: "unc_path", line: 5},
		{code: "short_name_path", line: 6},
	} {
		diagnostic := config.Diagnostics[index]
		if diagnostic.Code != want.code || diagnostic.Line != want.line || diagnostic.Source != configPath || !diagnostic.Recoverable {
			t.Fatalf("diagnostic[%d] = %#v, want stable %s line diagnostic", index, diagnostic, want.code)
		}
	}
}

func TestLoadProtectionConfigurationCollapsesDuplicateAndOverlappingEntriesDeterministically(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "protection.txt")
	content := "C:\\Work\\App\\cache\nC:\\Archive\nC:\\Work\\App2\n\\\\?\\c:\\work\\app\nC:\\WORK\\APP\n"
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOAL_PROTECTION_FILE", configPath)

	config := clean.LoadProtectionConfiguration()

	if config.LoadError != nil || len(config.Diagnostics) != 0 {
		t.Fatalf("config = %#v, want successfully loaded entries", config)
	}
	paths := config.Validator.UserProtectionPaths()
	if len(paths) != 3 || paths[0] != `C:\Archive` || paths[1] != `c:\work\app` || paths[2] != `C:\Work\App2` {
		t.Fatalf("active paths = %#v, want stable ancestor-only rules", paths)
	}
}

func TestLoadProtectionConfigurationDistinguishesMissingDefaultFromMissingOverride(t *testing.T) {
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	t.Setenv("FOAL_PROTECTION_FILE", "")

	defaultConfig := clean.LoadProtectionConfiguration()
	if defaultConfig.LoadError != nil || len(defaultConfig.Validator.UserProtectionPaths()) != 0 {
		t.Fatalf("missing default config = %#v, want harmless empty configuration", defaultConfig)
	}

	override := filepath.Join(t.TempDir(), "missing.txt")
	t.Setenv("FOAL_PROTECTION_FILE", "  "+override+"  ")
	overrideConfig := clean.LoadProtectionConfiguration()
	if overrideConfig.LoadError == nil ||
		overrideConfig.LoadError.Code != "protection_file_load_failed" ||
		overrideConfig.LoadError.Path != override ||
		overrideConfig.LoadError.Recoverable {
		t.Fatalf("missing override config = %#v, want non-recoverable selected-file error", overrideConfig)
	}

	empty := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOAL_PROTECTION_FILE", empty)
	emptyConfig := clean.LoadProtectionConfiguration()
	if emptyConfig.LoadError != nil || len(emptyConfig.Validator.UserProtectionPaths()) != 0 || len(emptyConfig.Diagnostics) != 0 {
		t.Fatalf("empty config = %#v, want valid empty configuration", emptyConfig)
	}
}

func TestLoadProtectionConfigurationFailsClosedForIncompleteUTF8Input(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "protection.txt")
	if err := os.WriteFile(configPath, []byte{'C', ':', '\\', 'W', 'o', 'r', 'k', '\n', 0xff}, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOAL_PROTECTION_FILE", configPath)

	config := clean.LoadProtectionConfiguration()

	if config.LoadError == nil || config.LoadError.Code != "protection_file_invalid_utf8" || config.LoadError.Recoverable {
		t.Fatalf("config = %#v, want non-recoverable UTF-8 load error", config)
	}
	if len(config.Validator.UserProtectionPaths()) != 0 {
		t.Fatalf("active paths = %#v, want none after incomplete load", config.Validator.UserProtectionPaths())
	}
}

func TestLoadProtectionConfigurationFailsClosedWhenSelectedPathIsUnreadableAsAFile(t *testing.T) {
	configPath := t.TempDir()
	t.Setenv("FOAL_PROTECTION_FILE", configPath)

	config := clean.LoadProtectionConfiguration()

	if config.LoadError == nil || config.LoadError.Code != "protection_file_load_failed" || config.LoadError.Path != configPath {
		t.Fatalf("config = %#v, want selected-file load failure", config)
	}
}
