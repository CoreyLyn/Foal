# Clean TUI uses dual-channel presentation for size vs deletion risk

The category-first Clean TUI needed clearer scannability for large reclaimable sizes without teaching users that "red means big." We decided that presentation uses two independent channels: **magnitude emphasis** on trusted byte tokens (neutral / amber from 100 MiB / orange from 1 GiB, never pure red for size) and **risk** via confirmation grouping, irreversible permanent warnings, and a selection footer notice when permanent work is selected. Per-row `perm`/`bin` prefixes were tried and removed as list noise; action detail stays on confirmation. Styling may apply restricted mid-line token styles after a plain-text frame is built so tests and copy keep plain oracles; `NO_COLOR` and weak terminals fall back to bold or plain without magnitude hues. This slice does not reorder by size, tint whole rows from magnitude, recolor execution progress, or change discovery, contracts, selection, or catalog-owned planned actions.

## Considered options

- **Single channel: red/yellow by size only** — rejected: collides with permanent-deletion danger language and overstates risk for large Recycle Bin work.
- **Single channel: risk color only** — rejected: leaves multi-GB rows easy to miss in a long category list.
- **Whole-line magnitude tint** — rejected: fights cursor reverse selection chrome and implies the entire row is hazardous.
- **Keep whole-line-only styling with no mid-line tokens** — rejected for this slice: byte-only emphasis needs a narrow token exception; plain frame remains authoritative.

## Consequences

- Implementers must not use pure red for gigabyte magnitude; red stays for irreversible / permanent warning emphasis.
- Preview lists do not prefix each row with planned-action markers; the permanent-selection notice and confirmation remain the risk cues.
- Display and scan order stay catalog-stable; size emphasis is visual, not sort order.
- A later presentation-only refinement added reliability-state marker colors and semantic line roles: success, attention, skipped, progress, and empty markers stay distinct from magnitude and irreversible-risk cues; failed/partial/canceled use attention rather than pure red. `NO_COLOR` preserves plain symbols and selection semantics. Stable path-free reason codes explain non-successful execution rows without forwarding raw issue messages or paths.
