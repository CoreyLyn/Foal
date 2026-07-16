package clean

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMatchJetBrainsProductVersionDir(t *testing.T) {
	cases := []struct {
		name        string
		wantApp     string
		wantVersion string
		wantOK      bool
	}{
		{"IntelliJIdea2024.1", ApplicationIntelliJIDEA, "2024.1", true},
		{"IdeaIC2020.1", ApplicationIntelliJIDEA, "2020.1", true},
		{"ideaic2023.2", ApplicationIntelliJIDEA, "2023.2", true},
		{"PyCharm2025.1", ApplicationPyCharm, "2025.1", true},
		{"PyCharmCE2024.3", ApplicationPyCharm, "2024.3", true},
		{"pycharmce2024.1", ApplicationPyCharm, "2024.1", true},
		{"IntelliJIdea2019.3", "", "", false},
		{"IntelliJIdea2020", "", "", false},
		{"IntelliJIdea2020.0", "", "", false},
		{"IntelliJIdea2024.1.1", "", "", false},
		{"IntelliJIdea2024.1-backup", "", "", false},
		{"MyIntelliJIdea2024.1", "", "", false},
		{"Rider2024.1", "", "", false},
		{"WebStorm2024.1", "", "", false},
		{"PyCharmEdu2024.1", "", "", false},
		{"Toolbox", "", "", false},
		{"ReSharper", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
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

func TestResolveJetBrainsIDECacheRootScopesOrdering(t *testing.T) {
	local := t.TempDir()
	jb := filepath.Join(local, "JetBrains")
	for _, name := range []string{
		"PyCharmCE2024.3",
		"IntelliJIdea2024.2",
		"PyCharm2024.1",
		"IdeaIC2024.1",
		"Rider2024.1", // ignored
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
	if len(scopes) != 4 {
		t.Fatalf("scopes = %#v, want 4 product roots", scopes)
	}
	// Product catalog order: IDEA editions by version, then PyCharm by version.
	wantNames := []string{"IdeaIC2024.1", "IntelliJIdea2024.2", "PyCharm2024.1", "PyCharmCE2024.3"}
	wantApps := []string{ApplicationIntelliJIDEA, ApplicationIntelliJIDEA, ApplicationPyCharm, ApplicationPyCharm}
	for i, scope := range scopes {
		if filepath.Base(scope.Path) != wantNames[i] {
			t.Fatalf("scopes[%d] = %q, want %q", i, scope.Path, wantNames[i])
		}
		if scope.Application != wantApps[i] {
			t.Fatalf("scopes[%d].Application = %q, want %q", i, scope.Application, wantApps[i])
		}
	}
}
