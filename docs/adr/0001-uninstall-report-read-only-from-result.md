# Uninstall human report renders read-only from the hardened uninstall Result

The `foal uninstall` human report renders directly from the already-hardened `uninstall.Result` read model and writes nothing to disk, deliberately diverging from `clean`, which projects a separate `PreviewReadModel` and writes a detailed candidate list companion file. We chose this because `uninstall.Result` is already the stable read model (hardened in #40) and `uninstall` is a preview-only, read-only command: a second projection would duplicate that contract, and a companion file would give a read-only review command a disk side effect.

## Considered options

- **Mirror clean's projection (`NewPreviewReadModel`) for symmetry** — rejected: duplicates the read model #40 just hardened, leaving two models to keep in sync.
- **Write a human-readable companion file like clean's detailed candidate list** — rejected: introduces a disk write into a read-only review command; the hardened `--json` output is already the full-detail overflow surface.

## Consequences

A future "make uninstall consistent with clean" refactor would reintroduce either a redundant projection or a disk write that breaks uninstall's read-only identity. Treat the divergence as intentional, not an oversight to be tidied up.
