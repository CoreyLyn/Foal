package clean

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

func TestIsGrokBuildUpdateResidueFileName(t *testing.T) {
	// Accept exact fixed names and anchored decimal diverted forms.
	accept := []string{
		"grok.exe.old",
		"agent.exe.old",
		"grok.exe.old.1234-1.old",
		"agent.exe.old.9-99.old",
		"grok.exe.old.0-0.old",
	}
	for _, name := range accept {
		if !isGrokBuildUpdateResidueFileName(name) {
			t.Errorf("accept %q", name)
		}
	}

	reject := []string{
		"",
		"Grok.exe.old",
		"GROK.EXE.OLD",
		"Agent.exe.old",
		"grok.exe",
		"agent.exe",
		"something.old",
		"grok.exe.old.bak",
		"grok.exe.rollback.bak",
		".rollback.bak",
		"grok.exe.old.-1.old",
		"grok.exe.old.1-.old",
		"grok.exe.old.1-2-3.old",
		"grok.exe.old.1a-2.old",
		"grok.exe.old.1-2a.old",
		"grok.exe.old..old",
		"nested/grok.exe.old",
		`nested\grok.exe.old`,
		"grok.exe.old. ",
		"other.exe.old",
	}
	for _, name := range reject {
		if isGrokBuildUpdateResidueFileName(name) {
			t.Errorf("reject %q", name)
		}
	}
}

func TestResolveGrokBuildHome_RootTable(t *testing.T) {
	fixture := t.TempDir()
	validRoot := filepath.Join(fixture, "custom-grok")
	if err := os.MkdirAll(validRoot, 0700); err != nil {
		t.Fatal(err)
	}
	defaultHome := filepath.Join(fixture, "user")
	defaultRoot := filepath.Join(defaultHome, ".grok")
	if err := os.MkdirAll(defaultRoot, 0700); err != nil {
		t.Fatal(err)
	}

	t.Run("unset uses USERPROFILE .grok", func(t *testing.T) {
		deps := grokBuildResidueDeps{
			lookupEnv: func(key string) (string, bool) {
				if key == "USERPROFILE" {
					return defaultHome, true
				}
				return "", false
			},
			userHomeDir: func() (string, error) { return "", errTestHome },
			joinPath:    filepath.Join,
			lstat:       os.Lstat,
		}
		root, status := resolveGrokBuildHome(deps, pathsafe.Validator{})
		if status != grokRootOK || root != defaultRoot {
			t.Fatalf("root=%q status=%v, want %q ok", root, status, defaultRoot)
		}
	})

	t.Run("valid absolute override", func(t *testing.T) {
		deps := grokBuildResidueDeps{
			lookupEnv: func(key string) (string, bool) {
				if key == "GROK_HOME" {
					return validRoot, true
				}
				return "", false
			},
			joinPath: filepath.Join,
			lstat:    os.Lstat,
		}
		root, status := resolveGrokBuildHome(deps, pathsafe.Validator{})
		if status != grokRootOK || root != filepath.Clean(validRoot) {
			t.Fatalf("root=%q status=%v", root, status)
		}
	})

	t.Run("blank override fails closed", func(t *testing.T) {
		deps := grokBuildResidueDeps{
			lookupEnv: func(key string) (string, bool) {
				if key == "GROK_HOME" {
					return "   ", true
				}
				// Default must not be used after blank override.
				if key == "USERPROFILE" {
					return defaultHome, true
				}
				return "", false
			},
			joinPath: filepath.Join,
			lstat:    os.Lstat,
		}
		root, status := resolveGrokBuildHome(deps, pathsafe.Validator{})
		if status != grokRootInvalidOverride || root != "" {
			t.Fatalf("root=%q status=%v, want invalid", root, status)
		}
	})

	t.Run("relative override fails closed", func(t *testing.T) {
		deps := grokBuildResidueDeps{
			lookupEnv: func(key string) (string, bool) {
				if key == "GROK_HOME" {
					return ".grok", true
				}
				return "", false
			},
			joinPath: filepath.Join,
			lstat:    os.Lstat,
		}
		gotRoot, status := resolveGrokBuildHome(deps, pathsafe.Validator{})
		if status != grokRootInvalidOverride || gotRoot != "" {
			t.Fatalf("root=%q status=%v, want invalid", gotRoot, status)
		}
	})

	t.Run("missing default is silent absence", func(t *testing.T) {
		missingHome := filepath.Join(fixture, "no-such-user")
		deps := grokBuildResidueDeps{
			lookupEnv: func(key string) (string, bool) {
				if key == "USERPROFILE" {
					return missingHome, true
				}
				return "", false
			},
			joinPath: filepath.Join,
			lstat:    os.Lstat,
		}
		root, status := resolveGrokBuildHome(deps, pathsafe.Validator{})
		if status != grokRootSilentAbsence || root != "" {
			t.Fatalf("root=%q status=%v", root, status)
		}
	})

	t.Run("dangerous override fails closed", func(t *testing.T) {
		deps := grokBuildResidueDeps{
			lookupEnv: func(key string) (string, bool) {
				if key == "GROK_HOME" {
					return `C:\Windows`, true
				}
				return "", false
			},
			joinPath: filepath.Join,
			lstat:    os.Lstat,
		}
		_, status := resolveGrokBuildHome(deps, pathsafe.Validator{})
		if status != grokRootInvalidOverride {
			t.Fatalf("status=%v, want invalid", status)
		}
	})

	t.Run("protected root", func(t *testing.T) {
		deps := grokBuildResidueDeps{
			lookupEnv: func(key string) (string, bool) {
				if key == "GROK_HOME" {
					return validRoot, true
				}
				return "", false
			},
			joinPath: filepath.Join,
			lstat:    os.Lstat,
		}
		root, status := resolveGrokBuildHome(deps, pathsafe.NewValidator([]string{validRoot}))
		if status != grokRootProtected || root != filepath.Clean(validRoot) {
			t.Fatalf("root=%q status=%v", root, status)
		}
	})
}

var errTestHome = errString("home unavailable")

type errString string

func (e errString) Error() string { return string(e) }

func TestGrokUpdateWitnessQuiet(t *testing.T) {
	root := t.TempDir()
	downloads := filepath.Join(root, "downloads")
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)

	deps := grokBuildResidueDeps{
		joinPath: filepath.Join,
		lstat:    os.Lstat,
		readDir:  os.ReadDir,
		now:      func() time.Time { return now },
	}

	t.Run("missing downloads is quiet", func(t *testing.T) {
		quiet, diag := grokUpdateWitnessQuiet(deps, root)
		if !quiet || diag != nil {
			t.Fatalf("quiet=%v diag=%v", quiet, diag)
		}
	})

	if err := os.MkdirAll(downloads, 0700); err != nil {
		t.Fatal(err)
	}

	t.Run("empty downloads is quiet", func(t *testing.T) {
		quiet, diag := grokUpdateWitnessQuiet(deps, root)
		if !quiet || diag != nil {
			t.Fatalf("quiet=%v diag=%v", quiet, diag)
		}
	})

	oldWitness := filepath.Join(downloads, "grok-1.2.3-windows")
	if err := os.WriteFile(oldWitness, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	oldTime := now.Add(-2 * time.Hour)
	if err := os.Chtimes(oldWitness, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	t.Run("old recognized file is quiet", func(t *testing.T) {
		quiet, diag := grokUpdateWitnessQuiet(deps, root)
		if !quiet || diag != nil {
			t.Fatalf("quiet=%v diag=%v", quiet, diag)
		}
	})

	// Exactly at one-hour boundary is quiet (exclusive window).
	boundary := filepath.Join(downloads, "grok-boundary")
	if err := os.WriteFile(boundary, []byte("b"), 0600); err != nil {
		t.Fatal(err)
	}
	boundaryTime := now.Add(-time.Hour)
	if err := os.Chtimes(boundary, boundaryTime, boundaryTime); err != nil {
		t.Fatal(err)
	}
	t.Run("exactly-at-boundary is quiet", func(t *testing.T) {
		quiet, diag := grokUpdateWitnessQuiet(deps, root)
		if !quiet || diag != nil {
			t.Fatalf("quiet=%v diag=%v", quiet, diag)
		}
	})

	// Unrelated recent file does not trip the gate.
	unrelated := filepath.Join(downloads, "notes.txt")
	if err := os.WriteFile(unrelated, []byte("n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(unrelated, now.Add(-time.Minute), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	t.Run("unrelated recent file is quiet", func(t *testing.T) {
		quiet, diag := grokUpdateWitnessQuiet(deps, root)
		if !quiet || diag != nil {
			t.Fatalf("quiet=%v diag=%v", quiet, diag)
		}
	})

	recent := filepath.Join(downloads, "grok-recent")
	if err := os.WriteFile(recent, []byte("r"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(recent, now.Add(-30*time.Minute), now.Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	t.Run("recent grok- witness is not quiet", func(t *testing.T) {
		quiet, diag := grokUpdateWitnessQuiet(deps, root)
		if quiet || diag != nil {
			t.Fatalf("quiet=%v diag=%v, want not quiet without diag", quiet, diag)
		}
	})
	_ = os.Remove(recent)

	future := filepath.Join(downloads, "grok-future")
	if err := os.WriteFile(future, []byte("f"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(future, now.Add(time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	t.Run("future timestamp is not quiet", func(t *testing.T) {
		quiet, diag := grokUpdateWitnessQuiet(deps, root)
		if quiet || diag != nil {
			t.Fatalf("quiet=%v diag=%v", quiet, diag)
		}
	})
}
