---
status: accepted
---

# Uninstall may request UAC; other commands stay non-elevating

Uninstall execution may request Windows administrator consent (UAC) when a selected traditional-desktop uninstall needs elevation, because many machine-wide installers cannot complete without it and Mole-like "remove the app completely" fails closed too often if Foal never elevates. This is an **Uninstall-only exception**: Clean, Purge, Analyze, Status, History, and other surfaces keep the existing product rule of **no automatic elevation**, reporting permission problems as skips with clear reasons. Elevation must be **disclosed before confirmation** by grouping apps that need admin, not sprung mid-batch without prior notice.

## Considered options

- **Never elevate (match Clean)** — rejected for Uninstall primary path: leaves a large share of real desktop uninstalls permanently skipped.
- **Product-wide automatic elevation** — rejected: expands risk and test surface for Clean/Purge without benefit to Foal's cleanup identity.
- **Prompt-only copy-paste admin command without Foal-initiated UAC** — rejected as the main policy: weaker UX for the chosen Mole-aligned uninstall job.

## Consequences

Agents and docs must stop treating "no automatic elevation" as universal once Uninstall execution ships; the accurate rule is **no elevation outside Uninstall execution**. Tests must cover non-elevated skips for Clean and elevated/grouped disclosure for Uninstall without assuming a single process token for all commands.
