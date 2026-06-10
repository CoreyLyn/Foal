# Bubble Tea v2 is the TUI shell framework

The interactive `fo`/`foal` shell was originally hand-written: byte-level key parsing, full-screen ANSI repaints, manual raw-mode and alternate-screen handling, and a custom Ctrl+C terminal-restore path. Closing its known gaps (clamped scrolling, terminal-size awareness, flicker-free rendering, color) would have meant rebuilding a TUI framework piece by piece, so we adopted `charm.land/bubbletea/v2` (with `bubbles/v2` and `lipgloss/v2`) and deleted the hand-written input, renderer, and signal-restore machinery. This deliberately ends the repo's near-zero-dependency state (previously only `golang.org/x/sys`) and raises the toolchain floor to Go 1.25.0, which bubbletea v2 requires.

Note the module path: v2 lives under `charm.land/...`, not `github.com/charmbracelet/...` — the GitHub path fails with a module path mismatch for v2 release tags.

## Considered options

- **Keep the zero-dependency hand-written shell** — rejected: scrolling clamp, resize handling, diff rendering, and styling are exactly what a framework provides; the in-tree key parser was already ~100 lines and still missed PgUp/PgDn, resize events, and Windows extended-key edge cases.
- **bubbletea v1 (works on Go 1.22, no toolchain bump)** — rejected: adopting the previous major in mid-2026 means migrating to v2 later anyway; v2 was GA with the full companion stack.

## Consequences

- All contributors and CI need Go ≥ 1.25 (or `GOTOOLCHAIN=auto`).
- Terminal restoration, raw mode, and the alternate screen are framework-owned; the TUI layer must not reintroduce manual console-mode handling.
- Ctrl+C arrives as a `KeyPressMsg`, not a signal — the `"ctrl+c"` Update case is load-bearing for the interrupt exit path (exit code 130).
- The TUI remains a read-only review surface over shared read models (see CONTEXT.md); the framework choice does not change that boundary.
