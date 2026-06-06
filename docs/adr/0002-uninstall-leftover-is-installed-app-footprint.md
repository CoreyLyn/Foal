# Uninstall leftover discovery surfaces installed-application footprint, not orphan residue

The `foal uninstall` leftover discovery (the `known_leftover_locations` provider) surfaces the filesystem footprint of applications Foal has *already discovered* (still installed) — directories under Roaming, Local, and ProgramData that strict whole-name matching ties to a discovered application — rather than orphaned residue of applications that are no longer installed. We chose this because matching against a known, discovered application name is a high-signal, low-false-positive association, while orphan detection (a directory that looks like application data with no matching installed application) is a heuristic, false-positive-heavy problem. This is why `Possible leftovers` can list paths for applications that are still installed: the term means "would likely remain after an uninstall," surfaced read-only for review, not "the application is already gone."

## Considered options

- **Detect orphan residue first (directories with no matching installed application)** — rejected for the first slice: it is the classic uninstall-cleaner value, but without a discovered application to anchor on it is heuristic and false-positive-heavy. Deferred to a later slice under a distinct term (e.g. `Orphaned residue`).
- **Do both (installed-app footprint and orphan residue) in one slice** — rejected: doubles the matching logic and the false-positive surface in the first slice. The two have different confidence profiles and deserve separate, independently verifiable slices.
- **Introduce a separate `Application footprint` concept distinct from `Possible leftovers`** — rejected: the read model and its tests already classify name-matched directories of discovered apps as possible leftovers, so a new section would add a JSON field and a dual-mode classifier for a distinction the precise `Possible leftovers` definition already carries.

## Consequences

`Possible leftovers` in `foal uninstall` will list paths for applications that are still installed, which reads as surprising unless you know the term means "would remain after an uninstall." A future orphan-residue slice must not overload `Possible leftovers`; it introduces its own term so the two confidence profiles stay distinct. Discovery stays read-only and preview-only: footprint paths are never deletion candidates, and `execution.allowed` stays `false` with `execution.actions` empty.
