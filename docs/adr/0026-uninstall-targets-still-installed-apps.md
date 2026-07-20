---
status: accepted
---

# Uninstall targets still-installed apps; orphans stay outside Uninstall execution

Foal's Uninstall command, when execution is designed, follows Mole's product split rather than a pure residual-cleaner or a full third-party uninstaller suite: **`uninstall` handles applications that are still installed** (remove the app plus high-confidence related file leftovers), while **already-uninstalled orphan residue stays on Clean-side / read-only Orphaned residue review** and must not be executed through Uninstall. We chose this because Mole's documented boundary is "use clean when the app is already uninstalled, and uninstall when the app is still installed," and Foal already separates **Possible leftovers** (installed-app footprint) from **Orphaned residue** (ADR 0002).

## Considered options

- **Residual-cleaner only (never run uninstallers)** — rejected as the primary job: useful later as Clean/orphan work, but not Mole-aligned Uninstall.
- **Full parity uninstaller suite (Store, winget, deep registry, force remove in v1)** — rejected for the first execution model: Windows risk and surface area explode before the traditional-desktop path is proven.
- **Keep Uninstall preview-only forever** — rejected as the long-term product decision once an execution model is designed; preview remains the safe default until flags/TUI confirmation authorize mutation.

## Consequences

Uninstall execution must not treat Orphaned residue, shared-state concerns, or unknown state as deletion targets. Clean must not gain "uninstall this still-installed app" as a category. ADR 0001 (read-only report from Result) remains valid for preview output; execution is a separate confirmed path, not a companion-file or report side effect. ADR 0002's discovery meaning for Possible leftovers stands; this ADR only opens a later confirmed-deletion use of that high-confidence class under a Confirmed leftover path set.
