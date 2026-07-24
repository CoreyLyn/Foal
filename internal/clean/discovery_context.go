package clean

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// discoveryContext is the package-private inject surface for category discovery
// and revalidation. Production fills real FS/env/clock; tests override roots,
// env values, now, and activity detectors. Public Options composition uses a
// single FixedRootDiscoveryOptions bag for the fixed-root cluster.
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
	// activityDetector maps canonical category id → activity observation.
	// When set, overrides the policy production detector for that category.
	activityDetector map[string]func(context.Context) fixedRootActivityState
}

func newProductionDiscoveryContext() discoveryContext {
	return discoveryContext{
		now:              time.Now(),
		lstat:            os.Lstat,
		readDir:          os.ReadDir,
		walkDir:          filepath.WalkDir,
		joinPath:         filepath.Join,
		getenv:           os.Getenv,
		rootOverride:     map[string]string{},
		envOverride:      map[string]string{},
		activityDetector: map[string]func(context.Context) fixedRootActivityState{},
	}
}

// FixedRootDiscoveryOptions injects roots, SystemRoot, clock, and activity
// detectors for the fixed-root category cluster (windows-temp, lghub-cache,
// thunder-update-download, windows-update-download-cache). Production leaves
// the zero value. Tests must use isolated roots and never read or mutate real
// machine-wide cache trees or live process/service state.
type FixedRootDiscoveryOptions struct {
	// Now overrides the current time for stability window calculations.
	// Test-only; production leaves it zero so time.Now() is used.
	Now time.Time
	// Roots maps canonical category id → absolute discovery root override.
	// Test-only; production leaves it nil/empty.
	Roots map[string]string
	// SystemRoot overrides the SystemRoot environment value used by categories
	// that resolve under %SystemRoot%. Test-only; production leaves it empty so
	// os.Getenv("SystemRoot") is read. Ignored for a category when Roots[id] is set.
	SystemRoot string
	// DetectLGHUBActivity reports LG HUB process/service activity. nil selects
	// the production platform detector.
	DetectLGHUBActivity func(context.Context) LGHUBActivityState
	// DetectThunderActivity reports Thunder process/service activity. nil selects
	// the production platform detector.
	DetectThunderActivity func(context.Context) ThunderUpdateDownloadActivityState
	// DetectWindowsUpdateServices reports Windows Update service-stack state.
	// nil selects the production platform detector.
	DetectWindowsUpdateServices func(context.Context) WindowsUpdateServicesState
}

// discoveryContextFromOptions builds the resolve-time context from Options.
func discoveryContextFromOptions(opts Options) discoveryContext {
	return discoveryContextFromFixedRootOptions(opts.FixedRootDiscoveryOptions)
}

// discoveryContextFromFixedRootOptions builds a context for discovery or
// identity revalidation from the consolidated fixed-root inject bag.
func discoveryContextFromFixedRootOptions(opts FixedRootDiscoveryOptions) discoveryContext {
	dc := newProductionDiscoveryContext()
	if !opts.Now.IsZero() {
		dc.now = opts.Now
	}
	for category, root := range opts.Roots {
		if trimmed := strings.TrimSpace(root); trimmed != "" && strings.TrimSpace(category) != "" {
			dc.rootOverride[category] = filepath.Clean(trimmed)
		}
	}
	// Match prior windows-temp / windows-update deps: only a non-blank SystemRoot
	// overrides getenv; whitespace-only falls through to the real environment.
	if sr := strings.TrimSpace(opts.SystemRoot); sr != "" {
		dc.envOverride["SystemRoot"] = sr
	}
	if opts.DetectLGHUBActivity != nil {
		detect := opts.DetectLGHUBActivity
		dc.activityDetector[CategoryLGHUBCache] = func(ctx context.Context) fixedRootActivityState {
			s := detect(ctx)
			return fixedRootActivityState{Status: fixedRootActivityStatus(s.Status), Message: s.Message}
		}
	}
	if opts.DetectThunderActivity != nil {
		detect := opts.DetectThunderActivity
		dc.activityDetector[CategoryThunderUpdateDownload] = func(ctx context.Context) fixedRootActivityState {
			s := detect(ctx)
			return fixedRootActivityState{Status: fixedRootActivityStatus(s.Status), Message: s.Message}
		}
	}
	if opts.DetectWindowsUpdateServices != nil {
		detect := opts.DetectWindowsUpdateServices
		dc.activityDetector[CategoryWindowsUpdateDownloadCache] = func(ctx context.Context) fixedRootActivityState {
			s := detect(ctx)
			return fixedRootActivityState{Status: fixedRootActivityStatus(s.Status), Message: s.Message}
		}
	}
	return dc
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

func (dc discoveryContext) hasActivityDetector(category string) bool {
	if dc.activityDetector == nil {
		return false
	}
	f, ok := dc.activityDetector[category]
	return ok && f != nil
}

func (dc discoveryContext) activityDetectorFor(category string) func(context.Context) fixedRootActivityState {
	if dc.activityDetector == nil {
		return nil
	}
	return dc.activityDetector[category]
}

// fixedRootOpts is a small builder for tests that need one category root + clock.
func fixedRootOpts(category, root string, now time.Time) FixedRootDiscoveryOptions {
	opts := FixedRootDiscoveryOptions{Now: now}
	if strings.TrimSpace(root) != "" {
		opts.Roots = map[string]string{category: root}
	}
	return opts
}
