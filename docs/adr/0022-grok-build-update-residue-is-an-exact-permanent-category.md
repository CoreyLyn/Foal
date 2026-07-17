# Grok Build update residue is an exact permanent category

## Status

Accepted. CLI tracer implementation shipped with issue #262.

## Context

CLI agent research found that Grok Build's user root mixes sessions, configuration, plugins, logs, downloaded binaries, and updater artifacts. Official updater source deliberately retains the current and one previous versioned download and protects fresh concurrent downloads. Whole `$GROK_HOME\downloads` cleanup would violate the product lifecycle and can break Unix installations that symlink to those files.

On Windows, replacing a locked `bin\grok.exe` or `bin\agent.exe` renames the old executable to an exact `.old` sibling. Grok best-effort deletes these backups on later updates; a running old image may keep one locked until the process exits. Local non-content metadata found one 138 MB residue, making the exact backup family a useful tracer without broadening into conversation or configuration cleanup.

## Decision

Register one product-scoped permanent category `grok-build-update-residue` with these boundaries:

- Resolve unset `GROK_HOME` to the current user's `%USERPROFILE%\.grok`. Accept an override only when non-blank, absolute, canonicalizable, and safe. Blank, relative, unavailable, reparse-point, dangerous, or protected roots fail closed without default/CWD fallback.
- Inspect only the direct `bin` child. Candidate names are lowercase exact `grok.exe.old`, `agent.exe.old`, or anchored `grok.exe.old.<pid>-<seq>.old` / `agent.exe.old.<pid>-<seq>.old` with decimal-only fields.
- Exclude every other `*.old`, `.rollback.bak`, directory, reparse point, download, staging payload, session, log, configuration, extension, and plugin artifact.
- Require `grok.exe` and `agent.exe` to be known idle before and after discovery. Running or unknown state skips the whole category.
- Treat any direct ordinary `$GROK_HOME\downloads\grok-*` file written within one hour as recent updater activity and skip the whole category. Missing downloads means no observed activity; unreadable directories or unknown relevant timestamps fail closed. Witness files never become candidates.
- Freshly resolve and revalidate the exact direct path and ordinary-file type immediately before mutation.
- Use planned action `delete_permanently`; CLI Execute requires per-run `--allow-permanent`, and TUI confirmation must disclose and authorize permanent deletion. Never fall back to Recycle Bin.
- CLI default Clean does no Grok probing. Enable through exact `grok-build-update-residue`, selection alias `cli-agents`, or `all`; do not include it in `dev-caches`. Clean TUI may eagerly measure and initially select a measurable candidate under the existing removable permanent-category rule.

## Consequences

- Foal can reclaim abandoned updater backups without treating Grok's home or downloads directory as cache.
- Concurrent or ambiguous update state produces a safe skip rather than partial cleanup.
- The category may false-skip when an unrelated future `grok-*` payload is recent; this is intentional and safer than version-specific updater attribution.
- Adding Windows staging payloads, versioned downloads, Grok logs, or other CLI agents requires separate evidence and product decisions.
