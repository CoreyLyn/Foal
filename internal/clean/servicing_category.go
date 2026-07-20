package clean

import "context"

// CategoryWinSxSComponentStore is the exact-selection-only Windows
// component-store servicing category. Its planned action is
// invoke_windows_servicing: it never yields a file candidate, byte estimate, or
// WinSxS path. Read-only component-store analysis is delegated to the Windows
// servicing stack through the injected ServicingGateway. See ADR 0029.
const CategoryWinSxSComponentStore = "winsxs_component_store"

// categoryResolverWindowsServicing marks a category whose work is a non-file
// Windows servicing operation rather than filesystem deletion. Servicing
// categories register an inert file resolver: candidate resolution, the eager
// preview, and single-category resolution never analyze WinSxS or trigger UAC.
const categoryResolverWindowsServicing categoryResolverKind = "windows-servicing"

type windowsServicingResolver struct{}

// resolve is intentionally inert. A servicing category produces no file
// candidates and no bytes, and must never contact the elevated helper from the
// shared category core: the eager preview / TUI entry and ordinary resolution
// must not analyze WinSxS or request elevation. Read-only analysis is a
// separate, plan-scoped step (appendServicingAnalysis) driven only by an exact
// CLI opt-in.
func (windowsServicingResolver) resolve(_ context.Context, _ Options, _ string, _ *categoryCoreResult) {
}

// windowsServicingCategoryEntry registers a Windows servicing category. Every
// servicing category is exact-selection-only, so callers cannot request UAC or
// component-store mutation through `all`, a group token, or TUI Select All.
func windowsServicingCategoryEntry(definition CleanupCategoryDefinition) categoryCatalogEntry {
	definition.SelectionPolicy = CategorySelectionPolicyExactOnly
	return categoryCatalogEntry{
		definition:   definition,
		resolverKind: categoryResolverWindowsServicing,
		resolver:     windowsServicingResolver{},
	}
}

// isServicingCategory reports whether a canonical category delegates to Windows
// servicing (planned action invoke_windows_servicing) instead of file deletion.
// Unknown identifiers report false.
func isServicingCategory(identifier string) bool {
	entry, ok := canonicalCategoryEntry(identifier)
	return ok && entry.definition.PlannedAction == PlannedActionInvokeWindowsServicing
}

// planServicingCategories returns the opted-in servicing categories in stable
// catalog order. Servicing categories are always exact-selection-only, so this
// is non-empty only when the run named the category exactly.
func planServicingCategories(plan CategoryPlan) []string {
	var servicing []string
	for _, id := range plan.Categories {
		if isServicingCategory(id) {
			servicing = append(servicing, id)
		}
	}
	return servicing
}
