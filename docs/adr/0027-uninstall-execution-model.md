---
status: accepted
---

# Uninstall execution model for traditional desktop apps

Foal will implement Uninstall execution as a **Mole-inspired, Windows-native** flow for **traditional desktop (registry Uninstall key) applications** first: multi-select batch, dry-run by default, one confirmation that discloses both uninstall work and leftovers, CLI and TUI sharing the same core (TUI never owns uninstaller, path safety, or deletion). Mechanism is hybrid: **prefer Official uninstaller invocation** (`QuietUninstallString` then interactive `UninstallString`); **Portable directory removal** only when there is no uninstall command and a trusted `InstallLocation` exists, with explicit permanent-deletion authorization. Leftover deletion is limited to the **Confirmed leftover path set** (high-confidence Possible leftovers only): confirmation freezes the upper bound; after the official uninstaller reports success (exit success is enough—no strict "registry gone" gate in v1), Foal revalidates and may delete only a subset, never expanding the set. User-profile leftovers prefer **Recycle Bin**; portable whole-tree removal is **permanent** and separately authorized; Protection rules apply deny-only as on Clean/Purge; full path-level **History** is required. Process stopping is opt-in per confirmation/run. First slice does **not** modify the registry, does **not** ship Force Removal for broken apps, does **not** include Store/MSIX or package-manager uninstall, and applies an **Uninstall hard exclusion** denylist (including Foal itself). CLI separates capabilities: `--execute`, process-stop authorization, and `--allow-permanent` (or TUI equivalents). Apps that need admin are **grouped and disclosed before confirmation**; elevation policy is ADR 0028.

## Considered options

- **Delete install trees as the primary mechanism (literal Mole `.app` delete)** — rejected on Windows: incomplete vs services/shared components and high mis-delete risk.
- **Two-phase leftover confirmation or leftovers-report-only** — rejected for primary UX: Mole-style one confirmation is the target; residual-only modes remain Clean/orphan concerns.
- **Post-uninstall deep leftover rescan that can add paths** — rejected: violates preview-first disclosure; confirmation list is a hard ceiling.
- **Registry leftover purge / Force Removal / Store+winget in v1** — deferred to keep the first executable path testable and conservative.

## Consequences

At decision time, shipped code remained preview-only until this model was implemented. The model is now implemented with shared core orchestration (not TUI-owned), Protection integration, History sessions distinct from Clean, authorization contract tests, and a non-expanding leftover set. Future Force Removal, registry cleanup, or package-manager paths need their own decisions and must not silently reuse Portable directory removal as a failure fallback.
