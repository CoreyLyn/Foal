package clean

import (
	"testing"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

func TestLookupPermanentIdentityValidatorPrefersOptionsOverRegistry(t *testing.T) {
	const category = "test_registry_category_only"
	// Isolate: ensure registry entry exists for this synthetic id, then Options wins.
	registerPermanentIdentityValidator(category, func(PermanentIdentityCandidate) (pathsafe.Reason, bool) {
		return pathsafe.Reason{Code: "from_registry", Message: "registry"}, false
	})
	t.Cleanup(func() {
		delete(permanentIdentityRegistry, category)
	})

	opts := Options{
		PermanentIdentityValidators: map[string]PermanentIdentityValidator{
			category: func(PermanentIdentityCandidate) (pathsafe.Reason, bool) {
				return pathsafe.Reason{Code: "from_options", Message: "options"}, false
			},
		},
	}
	validator := lookupPermanentIdentityValidator(opts, category)
	if validator == nil {
		t.Fatal("expected options validator")
	}
	reason, ok := validator(PermanentIdentityCandidate{Category: category})
	if ok || reason.Code != "from_options" {
		t.Fatalf("got ok=%v reason=%#v, want options override", ok, reason)
	}

	// Without Options entry for another id, registry is used.
	const registeredOnly = "test_registry_only_category"
	registerPermanentIdentityValidator(registeredOnly, func(PermanentIdentityCandidate) (pathsafe.Reason, bool) {
		return pathsafe.Reason{Code: "from_registry", Message: "registry"}, false
	})
	t.Cleanup(func() {
		delete(permanentIdentityRegistry, registeredOnly)
	})
	validator = lookupPermanentIdentityValidator(Options{}, registeredOnly)
	if validator == nil {
		t.Fatal("expected registry validator")
	}
	reason, ok = validator(PermanentIdentityCandidate{Category: registeredOnly})
	if ok || reason.Code != "from_registry" {
		t.Fatalf("got ok=%v reason=%#v, want registry", ok, reason)
	}

	// Missing category → nil (PathSafe-only).
	if lookupPermanentIdentityValidator(Options{}, "no_such_category") != nil {
		t.Fatal("missing category must be PathSafe-only")
	}
}
