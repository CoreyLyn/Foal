---
status: accepted
---

# Clean may delegate superseded display driver package removal

A real C: audit found 21 unused, superseded NVIDIA display driver packages (~46 GB) in the Windows driver store — each driver update adds a ~2.5 GB package and Windows never removes old ones. Removal requires administrator rights, and ADR 0029 requires any additional servicing operation to be a separate decision with its own capability. This ADR adds that capability as a third elevation exception alongside WinSxS servicing (ADR 0029) and Uninstall (ADR 0028).

## Decision

- New exact-selection-only category `superseded-display-drivers` with planned action `invoke_windows_servicing`. It is excluded from `all`, every group token, and TUI Select All, and starts unselected.
- **Analysis is non-elevated and in-process.** Dry-run and the TUI analysis action inventory third-party packages through documented SetupAPI calls (INF `[Version]` parsing, driver-store location, device `DEVPKEY_Device_DriverInfPath` across present and non-present devices). Analysis never requests UAC.
- **Eligibility** (see `docs/research/driver-store-superseded-display-packages.md`): published name `oem<digits>.inf`, Display class, family = original INF name + provider + class GUID, keep every package any device uses and the newest package per family (DriverVer date, then version, then published number); everything else in the family is a candidate. No rollback reserve is kept. Unparseable DriverVer fails the whole family closed; device-enumeration failure fails analysis closed.
- **Execution** requires `--execute`, exact selection, the existing per-run `--allow-servicing` (still independent of `--allow-permanent`), and UAC. The non-elevated coordinator performs a fresh analysis; the TUI additionally intersects it with the package set disclosed at confirmation, so execution never removes a package the user did not see. The coordinator sends only a bounded list of published names (each `^oem\d{1,6}\.inf$`, unique, at most 256) — a deliberate, narrow extension of ADR 0029's enum-only request, carried under protocol version 2. The elevated helper re-derives the inventory and policy itself and removes only the intersection by calling `SetupUninstallOEMInfW(name, 0, NULL)`.
- **Never**: `SUOI_FORCEDELETE`, PnPUtil, `DiUninstallDriver`, device uninstall, or direct deletion under `DriverStore`. Windows' documented refusal to remove a package any device was installed with is a second guard after Foal's own in-use exclusion.
- **Records** stay in the servicing operation record: per-package published name, original INF name, provider, DriverVer date/version, measured bytes, and outcome (`candidate`, `removed`, `in_use`, `not_eligible`, `failed`), plus an optional observed free-space delta. Package bytes are servicing evidence and never enter candidate, affected, Recycle Bin, or permanent byte totals. `restart_required` is always false (the API reports none). Cancellation is honored only before the helper request is sent; afterwards the coordinator waits for the actual outcome.

## Consequences

- AGENTS.md's elevation boundary lists three designed exceptions: WinSxS servicing, superseded display driver removal, and Uninstall.
- Users lose Device Manager rollback to removed versions and must download them again if needed; confirmation and help disclose this.
- Other driver classes, printer/audio/network packages, and Windows Update driver cleanup remain out of scope and need their own decisions.
