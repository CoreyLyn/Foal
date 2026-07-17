package clean

// permanentIdentityRegistry holds optional production category-owned permanent
// pre-mutation validators keyed by canonical category/rule ID. Existing
// permanent categories leave this empty and continue PathSafe-only through the
// shared permanent remover. Future exact-identity categories (for example
// product-scoped residue) register here without moving deletion ownership.
//
// Options.PermanentIdentityValidators overrides this map for tests.
var permanentIdentityRegistry = map[string]PermanentIdentityValidator{}

// registerPermanentIdentityValidator associates a category with an optional
// pre-mutation identity check. Not safe for concurrent registration after
// package init; intended for init-time category wiring.
func registerPermanentIdentityValidator(category string, validator PermanentIdentityValidator) {
	if category == "" || validator == nil {
		return
	}
	permanentIdentityRegistry[category] = validator
}

// lookupPermanentIdentityValidator resolves the category-owned validator for a
// permanent candidate. Options inject takes precedence over the production
// registry. A nil result means PathSafe-only composition.
func lookupPermanentIdentityValidator(opts Options, category string) PermanentIdentityValidator {
	if opts.PermanentIdentityValidators != nil {
		if validator, ok := opts.PermanentIdentityValidators[category]; ok {
			return validator
		}
	}
	return permanentIdentityRegistry[category]
}
