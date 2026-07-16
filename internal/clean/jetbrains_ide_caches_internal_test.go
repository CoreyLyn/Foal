package clean

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// jetbrainsCatalogPrefixCases is the private table for every anchored product
// prefix → logical application mapping in the expanded catalog (#209).
var jetbrainsCatalogPrefixCases = []struct {
	name        string
	wantApp     string
	wantVersion string
	wantOK      bool
}{
	// IntelliJ IDEA editions
	{"IntelliJIdea2024.1", ApplicationIntelliJIDEA, "2024.1", true},
	{"IdeaIC2020.1", ApplicationIntelliJIDEA, "2020.1", true},
	{"ideaic2023.2", ApplicationIntelliJIDEA, "2023.2", true},
	// PyCharm editions (longer CE prefix wins)
	{"PyCharm2025.1", ApplicationPyCharm, "2025.1", true},
	{"PyCharmCE2024.3", ApplicationPyCharm, "2024.3", true},
	{"pycharmce2024.1", ApplicationPyCharm, "2024.1", true},
	// Standard single-edition products
	{"WebStorm2024.1", ApplicationWebStorm, "2024.1", true},
	{"webstorm2020.1", ApplicationWebStorm, "2020.1", true},
	{"PhpStorm2024.2", ApplicationPhpStorm, "2024.2", true},
	{"RubyMine2024.1", ApplicationRubyMine, "2024.1", true},
	{"CLion2024.3", ApplicationCLion, "2024.3", true},
	{"DataGrip2024.1", ApplicationDataGrip, "2024.1", true},
	{"DataSpell2024.2", ApplicationDataSpell, "2024.2", true},
	{"GoLand2024.1", ApplicationGoLand, "2024.1", true},
	{"RustRover2024.3", ApplicationRustRover, "2024.3", true},
	{"Aqua2024.1", ApplicationAqua, "2024.1", true},
	{"MPS2024.1", ApplicationMPS, "2024.1", true},
	{"Writerside2024.1", ApplicationWriterside, "2024.1", true},
	// Fail-closed: version / decoy / deferred / non-catalog
	{"IntelliJIdea2019.3", "", "", false},
	{"IntelliJIdea2020", "", "", false},
	{"IntelliJIdea2020.0", "", "", false},
	{"IntelliJIdea2024.1.1", "", "", false},
	{"IntelliJIdea2024.1-backup", "", "", false},
	{"MyIntelliJIdea2024.1", "", "", false},
	{"Rider2024.1", "", "", false}, // #210 owns Rider
	{"PyCharmEdu2024.1", "", "", false},
	{"Fleet2024.1", "", "", false},
	{"AndroidStudio2024.1", "", "", false},
	{"WebIde2024.1", "", "", false},
	{"Toolbox", "", "", false},
	{"ReSharper", "", "", false},
	{"", "", "", false},
}

// jetbrainsCatalogProcessCases is the private table for every logical product's
// exact Windows launcher process names.
var jetbrainsCatalogProcessCases = []struct {
	application string
	executables []string
}{
	{ApplicationIntelliJIDEA, []string{"idea64.exe", "idea.exe"}},
	{ApplicationPyCharm, []string{"pycharm64.exe", "pycharm.exe"}},
	{ApplicationWebStorm, []string{"webstorm64.exe", "webstorm.exe"}},
	{ApplicationPhpStorm, []string{"phpstorm64.exe", "phpstorm.exe"}},
	{ApplicationRubyMine, []string{"rubymine64.exe", "rubymine.exe"}},
	{ApplicationCLion, []string{"clion64.exe", "clion.exe"}},
	{ApplicationDataGrip, []string{"datagrip64.exe", "datagrip.exe"}},
	{ApplicationDataSpell, []string{"dataspell64.exe", "dataspell.exe"}},
	{ApplicationGoLand, []string{"goland64.exe", "goland.exe"}},
	{ApplicationRustRover, []string{"rustrover64.exe", "rustrover.exe"}},
	{ApplicationAqua, []string{"aqua64.exe", "aqua.exe"}},
	{ApplicationMPS, []string{"mps64.exe", "mps.exe"}},
	{ApplicationWriterside, []string{"writerside64.exe", "writerside.exe"}},
}

func TestMatchJetBrainsProductVersionDir(t *testing.T) {
	for _, tc := range jetbrainsCatalogPrefixCases {
		policy, _, version, ok := matchJetBrainsProductVersionDir(tc.name)
		if ok != tc.wantOK {
			t.Fatalf("%q ok = %v, want %v", tc.name, ok, tc.wantOK)
		}
		if !tc.wantOK {
			continue
		}
		if policy.application != tc.wantApp {
			t.Fatalf("%q application = %q, want %q", tc.name, policy.application, tc.wantApp)
		}
		if version != tc.wantVersion {
			t.Fatalf("%q version = %q, want %q", tc.name, version, tc.wantVersion)
		}
	}
}

func TestJetBrainsCatalogProcessMappings(t *testing.T) {
	// Catalog application set must match process registry 1:1.
	catalogApps := jetbrainsIDEApplicationIDs()
	if len(catalogApps) != len(jetbrainsCatalogProcessCases) {
		t.Fatalf("catalog apps = %d, process cases = %d", len(catalogApps), len(jetbrainsCatalogProcessCases))
	}
	for i, app := range catalogApps {
		if jetbrainsCatalogProcessCases[i].application != app {
			t.Fatalf("process case[%d] = %q, want catalog order %q", i, jetbrainsCatalogProcessCases[i].application, app)
		}
	}

	for _, tc := range jetbrainsCatalogProcessCases {
		def, ok := developerApplicationDefinition(tc.application)
		if !ok {
			t.Fatalf("missing developer application definition for %q", tc.application)
		}
		if len(def.executables) != len(tc.executables) {
			t.Fatalf("%q executables = %#v, want %#v", tc.application, def.executables, tc.executables)
		}
		for i, exe := range tc.executables {
			if def.executables[i] != exe {
				t.Fatalf("%q executables[%d] = %q, want %q", tc.application, i, def.executables[i], exe)
			}
		}
		// Each launcher classifies only this product as running.
		for _, exe := range tc.executables {
			states := detectSupportedApplications(context.Background(), func(context.Context) processSnapshot {
				return processSnapshot{Names: []string{exe}}
			})
			for _, state := range states {
				if state.Application == tc.application {
					if state.State != RunningApplicationStateRunning {
						t.Fatalf("%q via %q = %#v, want running", tc.application, exe, state)
					}
					continue
				}
				// Other JetBrains products must remain idle.
				for _, other := range jetbrainsCatalogProcessCases {
					if other.application == state.Application && state.State != RunningApplicationStateIdle {
						t.Fatalf("%q running leaked to %q via %q", tc.application, state.Application, exe)
					}
				}
			}
		}
	}

	// Category runningApplications must list every catalog identity.
	entry, ok := canonicalCategoryEntry(DevCacheCategoryJetBrainsIDECaches)
	if !ok {
		t.Fatal("jetbrains-ide-caches missing from catalog")
	}
	if len(entry.runningApplications) != len(catalogApps) {
		t.Fatalf("runningApplications = %#v, want %d entries", entry.runningApplications, len(catalogApps))
	}
	for i, app := range catalogApps {
		if entry.runningApplications[i] != app {
			t.Fatalf("runningApplications[%d] = %q, want %q", i, entry.runningApplications[i], app)
		}
	}
}

func TestJetBrainsCatalogPrefixesCoverEveryPolicy(t *testing.T) {
	// Every policy prefix must match as an anchored product-version directory.
	for _, policy := range jetbrainsIDEProductPolicies {
		for _, prefix := range policy.prefixes {
			dir := prefix + "2024.1"
			got, _, version, ok := matchJetBrainsProductVersionDir(dir)
			if !ok || got.application != policy.application || version != "2024.1" {
				t.Fatalf("prefix %q dir %q => app=%q version=%q ok=%v", prefix, dir, got.application, version, ok)
			}
		}
	}
}

func TestIsJetBrainsSupportedVersion(t *testing.T) {
	valid := []string{"2020.1", "2020.2", "2024.1", "2025.3"}
	invalid := []string{"", "2020", "2019.3", "2020.0", "2020.01", "2024.1.1", "2024.1a", "EAP", "20.1"}
	for _, v := range valid {
		if !isJetBrainsSupportedVersion(v) {
			t.Fatalf("%q should be valid", v)
		}
	}
	for _, v := range invalid {
		if isJetBrainsSupportedVersion(v) {
			t.Fatalf("%q should be invalid", v)
		}
	}
}

func TestCompareJetBrainsVersions(t *testing.T) {
	if compareJetBrainsVersions("2024.1", "2024.2") >= 0 {
		t.Fatal("2024.1 should be before 2024.2")
	}
	if compareJetBrainsVersions("2023.3", "2024.1") >= 0 {
		t.Fatal("2023.3 should be before 2024.1")
	}
	if compareJetBrainsVersions("2024.1", "2024.1") != 0 {
		t.Fatal("equal versions")
	}
}

func TestDiscoverJetBrainsIDECacheChildrenAllowlistOnly(t *testing.T) {
	parent := t.TempDir()
	productRoot := filepath.Join(parent, "IntelliJIdea2024.1")
	for _, name := range []string{"caches", "index", "LocalHistory", "plugins", "log", "tmp"} {
		if err := os.MkdirAll(filepath.Join(productRoot, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(productRoot, "not-a-dir"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	children := discoverJetBrainsIDECacheChildren(context.Background(), productRoot)
	if len(children) != 2 {
		t.Fatalf("children = %#v, want caches+index only", children)
	}
	if filepath.Base(children[0]) != "caches" || filepath.Base(children[1]) != "index" {
		t.Fatalf("order/names = %#v", children)
	}
}

func TestDiscoverJetBrainsIDECacheChildrenNoResharperHost(t *testing.T) {
	// Standard-layout products must never inherit Rider's resharper-host child.
	parent := t.TempDir()
	for _, dir := range []string{"WebStorm2024.1", "GoLand2024.1", "CLion2024.1"} {
		root := filepath.Join(parent, dir)
		for _, name := range []string{"caches", "index", "resharper-host"} {
			if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
				t.Fatal(err)
			}
		}
		children := discoverJetBrainsIDECacheChildren(context.Background(), root)
		if len(children) != 2 {
			t.Fatalf("%s children = %#v, want caches+index only", dir, children)
		}
		for _, child := range children {
			if filepath.Base(child) == "resharper-host" {
				t.Fatalf("%s leaked resharper-host", dir)
			}
		}
	}
}

func TestResolveJetBrainsIDECacheRootScopesOrdering(t *testing.T) {
	local := t.TempDir()
	jb := filepath.Join(local, "JetBrains")
	for _, name := range []string{
		"PyCharmCE2024.3",
		"IntelliJIdea2024.2",
		"PyCharm2024.1",
		"IdeaIC2024.1",
		"WebStorm2024.1",
		"GoLand2023.3",
		"GoLand2024.1",
		"Rider2024.1", // ignored until #210
		"Fleet2024.1", // non-standard architecture
	} {
		if err := os.MkdirAll(filepath.Join(jb, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	scopes := resolveJetBrainsIDECacheRootScopes(devCachePathDependencies{
		lookupEnv: func(key string) (string, bool) {
			if key == "LOCALAPPDATA" {
				return local, true
			}
			return "", false
		},
		joinPath: filepath.Join,
	})
	// Product catalog order: IDEA editions by version, PyCharm by version,
	// then WebStorm, then GoLand versions ascending.
	wantNames := []string{
		"IdeaIC2024.1", "IntelliJIdea2024.2",
		"PyCharm2024.1", "PyCharmCE2024.3",
		"WebStorm2024.1",
		"GoLand2023.3", "GoLand2024.1",
	}
	wantApps := []string{
		ApplicationIntelliJIDEA, ApplicationIntelliJIDEA,
		ApplicationPyCharm, ApplicationPyCharm,
		ApplicationWebStorm,
		ApplicationGoLand, ApplicationGoLand,
	}
	if len(scopes) != len(wantNames) {
		t.Fatalf("scopes = %#v, want %d product roots", scopes, len(wantNames))
	}
	for i, scope := range scopes {
		if filepath.Base(scope.Path) != wantNames[i] {
			t.Fatalf("scopes[%d] = %q, want %q", i, scope.Path, wantNames[i])
		}
		if scope.Application != wantApps[i] {
			t.Fatalf("scopes[%d].Application = %q, want %q", i, scope.Application, wantApps[i])
		}
	}
}
