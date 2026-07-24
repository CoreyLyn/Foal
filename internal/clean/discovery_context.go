package clean

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// discoveryContext is the package-private inject surface for category discovery
// and revalidation. Production fills real FS/env/clock; tests override roots,
// env values, and now. Public Options composition stays free of per-category
// DiscoveryOptions bags as categories migrate onto this context.
type discoveryContext struct {
	now      time.Time
	lstat    func(string) (os.FileInfo, error)
	readDir  func(string) ([]fs.DirEntry, error)
	walkDir  func(string, fs.WalkDirFunc) error
	joinPath func(...string) string
	getenv   func(string) string

	// rootOverride maps canonical category id → absolute discovery root.
	// When set, category root resolution skips env/default derivation.
	rootOverride map[string]string
	// envOverride maps env var name → value (for example SystemRoot).
	// Empty string is a deliberate blank override (fail closed), distinct from
	// a missing key which falls through to getenv.
	envOverride map[string]string
}

func newProductionDiscoveryContext() discoveryContext {
	return discoveryContext{
		now:          time.Now(),
		lstat:        os.Lstat,
		readDir:      os.ReadDir,
		walkDir:      filepath.WalkDir,
		joinPath:     filepath.Join,
		getenv:       os.Getenv,
		rootOverride: map[string]string{},
		envOverride:  map[string]string{},
	}
}

// discoveryContextFromOptions builds the resolve-time context. Bridges remaining
// public *DiscoveryOptions fields until those categories migrate fully and the
// fields are removed from Options.
func discoveryContextFromOptions(opts Options) discoveryContext {
	dc := newProductionDiscoveryContext()
	// windows-temp bridge (PR1): public Options field still accepted.
	wt := opts.WindowsTempDiscoveryOptions
	if !wt.Now.IsZero() {
		dc.now = wt.Now
	}
	if root := strings.TrimSpace(wt.Root); root != "" {
		dc.rootOverride[CategoryWindowsTemp] = filepath.Clean(root)
	}
	// Match prior windows-temp deps: only a non-blank SystemRoot overrides getenv;
	// whitespace-only falls through to the real environment.
	if sr := strings.TrimSpace(wt.SystemRoot); sr != "" {
		dc.envOverride["SystemRoot"] = sr
	}
	return dc
}

// discoveryContextFromWindowsTempOptions builds a context for identity
// revalidation that only needs the windows-temp inject bag (still carried on
// CategoryIdentityCandidate until the identity bag is slimmed).
func discoveryContextFromWindowsTempOptions(opts WindowsTempDiscoveryOptions) discoveryContext {
	return discoveryContextFromOptions(Options{WindowsTempDiscoveryOptions: opts})
}

func (dc discoveryContext) env(name string) (string, bool) {
	if dc.envOverride != nil {
		if v, ok := dc.envOverride[name]; ok {
			return v, true
		}
	}
	if dc.getenv == nil {
		return "", false
	}
	return dc.getenv(name), false
}

func (dc discoveryContext) categoryRootOverride(category string) (string, bool) {
	if dc.rootOverride == nil {
		return "", false
	}
	root, ok := dc.rootOverride[category]
	if !ok || strings.TrimSpace(root) == "" {
		return "", false
	}
	return root, true
}
