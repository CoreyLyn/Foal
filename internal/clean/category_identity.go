package clean

import "github.com/CoreyLyn/Foal/internal/core/pathsafe"

// CategoryIdentityCandidate is the fresh candidate context an action-neutral,
// category-owned pre-mutation validator may inspect immediately before any
// mutation. Unlike PermanentIdentityCandidate it is not tied to permanent
// deletion: it carries the candidate's Planned action so the same validator
// contract can guard a Recycle Bin move. It is not an authorization token and
// never expands the candidate set.
type CategoryIdentityCandidate struct {
	Path          string
	Bytes         int64
	Category      string
	PlannedAction string
	// nvidiaDiscovery, lghubDiscovery, thunderUpdateDownloadDiscovery, and
	// validator carry the injects a category-owned action-neutral validator
	// needs to repeat its fresh proof. They are package-internal and set by
	// shared Execute from Options so tests exercise the same seams as
	// production. Categories that do not use them ignore them.
	nvidiaDiscovery                NVIDIAInstallerCacheDiscoveryOptions
	lghubDiscovery                 LGHUBCacheDiscoveryOptions
	thunderUpdateDownloadDiscovery ThunderUpdateDownloadDiscoveryOptions
	windowsTempDiscovery           WindowsTempDiscoveryOptions
	validator                      pathsafe.Validator
}

// CategoryIdentityValidator re-checks, immediately before mutation, that a
// candidate still has the category-specific identity that authorized discovery.
// It is action-neutral: the same contract guards Recycle Bin moves and any other
// action. It must not mutate the filesystem or own the mutation. ok=false is a
// recoverable skip that preserves the candidate's original planned action and
// never falls back to a different (for example permanent) action.
type CategoryIdentityValidator func(candidate CategoryIdentityCandidate) (pathsafe.Reason, bool)

// categoryIdentityRegistry holds optional production action-neutral category
// pre-mutation validators keyed by canonical category/rule ID. Categories
// without an entry keep PathSafe-only composition through the shared mutation
// path. Options.CategoryIdentityValidators overrides this map for tests.
var categoryIdentityRegistry = map[string]CategoryIdentityValidator{}

// registerCategoryIdentityValidator associates a category with an action-neutral
// pre-mutation identity check. Not safe for concurrent registration after
// package init; intended for init-time category wiring.
func registerCategoryIdentityValidator(category string, validator CategoryIdentityValidator) {
	if category == "" || validator == nil {
		return
	}
	categoryIdentityRegistry[category] = validator
}

// lookupCategoryIdentityValidator resolves the action-neutral category-owned
// validator for a candidate. Options injects take precedence over the production
// registry. A nil result means PathSafe-only composition.
func lookupCategoryIdentityValidator(opts Options, category string) CategoryIdentityValidator {
	if opts.CategoryIdentityValidators != nil {
		if validator, ok := opts.CategoryIdentityValidators[category]; ok {
			return validator
		}
	}
	return categoryIdentityRegistry[category]
}
