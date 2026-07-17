# Clean TUI uses dual-channel presentation for size vs deletion risk

The category-first Clean TUI needed clearer scannability for large reclaimable sizes without teaching users that "red means big." We decided that presentation uses two independent channels: **magnitude emphasis** on trusted byte tokens (neutral / amber from 100 MiB / orange from 1 GiB, never pure red for size) and **risk** via confirmation grouping, irreversible permanent warnings, compact `perm`/`bin` planned-action markers, and a selection footer notice when permanent work is selected. Styling may apply restricted mid-line token styles after a plain-text frame is built so tests and copy keep plain oracles; `NO_COLOR` and weak terminals fall back to bold or plain without magnitude hues. This slice does not reorder by size, tint whole rows from magnitude, recolor execution progress, or change discovery, contracts, selection, or catalog-owned planned actions.

## Considered options

- **Single channel: red/yellow by size only** — rejected: collides with permanent-deletion danger language and overstates risk for large Recycle Bin work.
- **Single channel: risk color only** — rejected: leaves multi-GB rows easy to miss in a long category list.
- **Whole-line magnitude tint** — rejected: fights cursor reverse selection chrome and implies the entire row is hazardous.
- **Keep whole-line-only styling with no mid-line tokens** — rejected for this slice: byte-only emphasis needs a narrow token exception; plain frame remains authoritative.

## Consequences

- Implementers must not use pure red for gigabyte magnitude; red stays for irreversible / permanent warning emphasis.
- Preview markers project catalog planned actions only; they never authorize cleanup or choose the action.
- Display and scan order stay catalog-stable; size emphasis is visual, not sort order.
- Future result-state palettes (e.g. success green) need a separate decision so they do not collide with magnitude orange or risk red.
