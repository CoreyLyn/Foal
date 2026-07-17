# Plan: Grok Build update residue cleanup

## Status

P1 specification accepted after a `grill-with-docs` session. CLI tracer shipped via issue #262 (`grok-build-update-residue` opt-in permanent category). `cli-agents` selection alias, Clean TUI eager/permanent selection, and docs alignment shipped via issue #263.

## Goal

Add one narrow, product-scoped Clean category that permanently deletes abandoned Windows executable backups created by the Grok Build updater, without treating Grok's downloads, sessions, configuration, logs, or extensions as cache.

The canonical category is `grok-build-update-residue`, label `Grok Build update residue`, report group `Developer tools`, eligibility `opt-in`, running-application policy `distinctive-process-must-be-idle`, and planned action `delete_permanently`.

## Evidence

- Grok's official updater downloads versioned binaries under `$GROK_HOME\downloads`, deliberately keeps current + one previous, and protects fresh concurrent downloads. Whole-directory cleanup is unsafe.
- On Windows, the updater renames locked `bin\grok.exe` / `bin\agent.exe` to exact `.old` siblings and best-effort removes those backups on later updates. Locked running images may leave the backup behind.
- Local non-content metadata found `bin\grok.exe.old` at about 138 MB. No session, prompt, credential, or configuration content was read.

Primary evidence and exact source links are recorded in [CLI agent local artifacts](../research/cli-agent-local-artifacts.md). Category decisions are fixed by [ADR 0021](../adr/0021-cli-agent-cleanup-uses-product-scoped-categories.md) and [ADR 0022](../adr/0022-grok-build-update-residue-is-an-exact-permanent-category.md).

## Functional contract

### Root resolution

- When `GROK_HOME` is unset, resolve only the current user's `%USERPROFILE%\.grok`.
- When `GROK_HOME` is set, accept it only when non-blank, absolute, canonicalizable, and safe.
- Blank, relative, unavailable, dangerous, protected, or reparse-point roots fail closed. Do not fall back to the default root or CWD after an invalid override.
- Inspect only the resolved root's direct `bin` and `downloads` children. Do not query the registry, run Grok, inspect projects, or search other drives/locations.
- Missing root or missing `bin` is silent absence. Invalid or unreadable state that prevents proving safety is a recoverable category skip/diagnostic, not authorization to broaden discovery.

### Candidate allowlist

Only direct, ordinary, non-reparse files in `$GROK_HOME\bin` with one of these lowercase names are candidates:

- `grok.exe.old`
- `agent.exe.old`
- `grok.exe.old.<pid>-<seq>.old`
- `agent.exe.old.<pid>-<seq>.old`

For generated siblings, `<pid>` and `<seq>` are non-empty decimal digit sequences and the match is anchored to the complete basename.

Never include generic `*.old`, `.rollback.bak`, directories, nested files, current executables, plugins, downloads, staging payloads, sessions, logs, credentials, configuration, or extensions. Zero exact matches means an empty category; there is no whole-`bin` fallback.

### Running-process gate

- Add one logical application identity for Grok Build covering both `grok.exe` and `agent.exe`.
- Both process names must be known idle before candidate discovery and again after discovery.
- Running or unknown state at either observation skips the whole category. Foal never stops either process.
- Use the existing shared running-application projection so CLI JSON, preview rows, diagnostics, and TUI remain path-free where required.

### Update-quiet gate

- Inspect direct ordinary files in `$GROK_HOME\downloads` only as update-activity witnesses, never as candidates.
- If any direct filename begins with lowercase `grok-` and its last-write time is within one hour of the injected current time, skip the whole category.
- A future timestamp is not proven quiet and therefore skips.
- A missing `downloads` directory means no observed update activity.
- An unreadable existing directory, unreadable relevant metadata, or unknown timestamp skips the whole category.
- The gate intentionally accepts false skips so future version/architecture/temp suffixes do not create an updater race.

### Preview and selection

- Default CLI Clean does not resolve Grok paths, inspect `downloads`, or detect Grok processes.
- Enable through `--opt-in grok-build-update-residue`, `--opt-in cli-agents`, or `--opt-in all`.
- `cli-agents` is a selection alias that expands to independently registered CLI-agent categories in catalog order. It is not a category and owns no resolver, candidates, or deletion policy.
- Do not include this category in `dev-caches`; updater residue is not cache.
- Clean TUI eager preview resolves the category through the shared category seam. When safely measurable, it starts selected under the existing permanent-category rule and remains independently removable.
- Default TUI rows and category status remain path-free. Merely browsing or selecting writes no History and performs no mutation.

### Execute and results

- Execute receives the frozen category identifier, but re-resolves the root, both gates, candidates, and Protection immediately before mutation. Preview paths are never trusted.
- Immediately before each deletion, verify the canonical root/direct `bin` parent, exact lowercase basename, ordinary non-reparse file type, and Protection state again.
- Planned and actual action is `delete_permanently`. CLI requires per-run `--allow-permanent`; TUI confirmation must disclose and authorize permanent deletion. Missing authorization skips candidates and never falls back to Recycle Bin.
- Independent file failures are isolated and reported through normal item-level Result/History semantics. Cancellation remains responsive and promises no rollback.
- Successful bytes count as permanently deleted bytes; do not claim secure erasure or guaranteed equal physical free-space gain.

## Architecture seam

Implement a dedicated private category resolver rather than forcing this policy through `developerCacheEntry` or `applicationCachePolicies`:

- The category has a product-specific root override, two-stage process gate, update-activity witness directory, and exact file candidates rather than a whole cache root.
- Register its path-free metadata in `canonicalCategoryEntries` and keep resolution behind `resolveCategoryCore`, so Dry-run, Clean TUI, and Execute share fresh behavior.
- Add injected dependencies for environment/home lookup, filesystem metadata/enumeration, process observation, and current time. Tests must never inspect the developer's real `.grok` root.
- Reuse shared Protection, PathSafe, measurement, planned-action, permanent-authorization, Result, History, and cancellation machinery. Do not duplicate deletion logic in CLI/TUI.

## Acceptance criteria

1. Catalog exposes exactly one executable category with the metadata specified above; permanent/recycle-bin matrix invariants and documentation counts are updated.
2. Exact category, `cli-agents`, and `all` select it deterministically; `dev-caches` does not. Invalid opt-ins remain rejected by the existing contract.
3. Default CLI Clean performs zero Grok root/process/update-witness work.
4. Unset `GROK_HOME` uses `%USERPROFILE%\.grok`; valid absolute override works; blank/relative/unsafe override never falls back or scans CWD.
5. Only the four anchored filename forms become independent candidates. Case variants, malformed numeric fields, `.rollback.bak`, nested decoys, directories, reparse points, and unrelated `.old` files remain untouched.
6. Pre/post running or unknown `grok.exe` / `agent.exe` state skips the whole category with recoverable evidence.
7. A recent/future/unknown `downloads\grok-*` timestamp or unreadable existing witness directory skips the whole category; old witnesses and a missing directory do not become candidates or add bytes.
8. Dry-run and TUI preview report measured permanent candidates without mutation. TUI initial selection and permanent-confirmation disclosure follow shared catalog behavior.
9. Execute without permanent authorization deletes nothing. Authorized Execute fresh-resolves and deletes only still-valid allowlisted files; replacement, reparse, Protection, or gate changes fail closed.
10. JSON and History preserve category, planned/actual permanent action, affected counts/bytes, skips, diagnostics, cancellation, and local failures without new path leakage in path-free TUI surfaces.
11. Full tests and `git diff --check` pass; README, AGENTS, deletion-policy plan, help text, and locked matrix tests remain aligned.

## Test matrix

- Root table: unset, valid absolute override, blank, whitespace, relative, missing, unreadable, dangerous, protected, root/bin reparse.
- Filename table: both exact names, numeric generated siblings, empty/non-digit/extra suffixes, case variants, rollback backup, generic old file, nested decoy, directory/reparse decoy.
- Process transitions: idle→idle, running→n/a, unknown→n/a, idle→running, idle→unknown for both executable names.
- Quiet witness: missing directory, empty, old recognized file, exactly-at-boundary, recent, future, unrelated recent file, unreadable directory, unreadable metadata.
- Surfaces: exact/group/all selection, dev-caches exclusion, CLI default no-probe, eager TUI selected/removable state, permanent confirmation.
- Execution: unauthorized, authorized success, per-file failure isolation, pre-delete replacement/reparse/protection, cancellation, Result/History action fidelity.

## Out of scope

- Cleaning whole `$GROK_HOME`, whole `downloads`, current/previous versioned binaries, `grok-windows-<arch>.exe` staging payloads, `.rollback.bak`, logs, sessions, memory, auth, configuration, plugins, skills, agents, or marketplace data.
- Running `grok update`, invoking installers, deleting/stopping processes, automatic elevation, Recycle Bin fallback, or secure erasure.
- Claude Code, Codex CLI, Antigravity CLI, or Gemini CLI catalog entries. Their current research remains evidence only.
- A generic CLI-agent cleanup resolver or execution category. Future agents require independent categories and proof.

## Likely implementation slices

1. Add catalog/application identity/selection-alias metadata and matrix tests.
2. Add the dedicated root, filename, process, and update-quiet resolver with hermetic unit tests.
3. Connect shared Dry-run/Execute/Protection/permanent-action Result and History behavior with integration tests.
4. Verify Clean TUI eager preview, initial selection, confirmation, and path-free rendering.
5. Align README, AGENTS, help text, deletion-policy plan, and canonical matrix counts.
