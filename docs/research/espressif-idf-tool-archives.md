# ESP-IDF tool archives (Clean eligibility)

Status: researched 2026-09-26 against ESP-IDF `v5.5.3` and the ESP-IDF
Installation Manager (EIM, idf-im-ui `5cf0db3`). Evidence for the
`espressif-tool-archives` Clean category.

## Decision

| Scope | Decision | Planned action |
| --- | --- | --- |
| Direct archive files in the `dist` download folder of a marked ESP-IDF tools directory, quiet for 24 hours | **Ship permanent (opt-in, `dev-caches`)** | `delete_permanently` |
| `tools` (installed tools), `python_env`, `idf-env.json`, subdirectories, the `dist` root | **Excluded** | — |

## Evidence

- `IDF_TOOLS_PATH` defaults to `~/.espressif`
  ([idf_tools.py#L96](https://github.com/espressif/esp-idf/blob/v5.5.3/tools/idf_tools.py#L96)).
  `dist` holds downloaded archives and `tools/<name>/<version>` holds installed
  tools ([idf-tools.rst#L33-L45](https://github.com/espressif/esp-idf/blob/v5.5.3/docs/en/api-guides/tools/idf-tools.rst#L33-L45),
  [#L64-L73](https://github.com/espressif/esp-idf/blob/v5.5.3/docs/en/api-guides/tools/idf-tools.rst#L64-L73)).
- Download: the file name comes from `rename_dist` or the URL basename; an
  existing archive is reused when its size and SHA-256 match, otherwise it is
  deleted and downloaded again; downloads are written to `<name>.tmp` and
  renamed ([idf_tools.py#L1207-L1286](https://github.com/espressif/esp-idf/blob/v5.5.3/tools/idf_tools.py#L1207-L1286)).
- `unpack()` supports `.zip`, `.tar.gz`, `.tgz`, `.tar.xz`, and `.tar.bz2`
  ([#L525-L539](https://github.com/espressif/esp-idf/blob/v5.5.3/tools/idf_tools.py#L525-L539));
  v5.5.3 Windows tools ship as `.zip` and `.tar.xz`, cross-platform tools as
  `.tar.gz` ([tools.json#L229](https://github.com/espressif/esp-idf/blob/v5.5.3/tools/tools.json#L229),
  [#L298](https://github.com/espressif/esp-idf/blob/v5.5.3/tools/tools.json#L298),
  [#L935](https://github.com/espressif/esp-idf/blob/v5.5.3/tools/tools.json#L935)).
- Installed tools do not need their archives; a missing archive is downloaded
  again when a tool is installed.
- `idf_tools.py uninstall --remove-archives` keeps only the archives of the
  current IDF version's installed tools
  ([#L3189-L3219](https://github.com/espressif/esp-idf/blob/v5.5.3/tools/idf_tools.py#L3189-L3219)).
  Its basename match ignores `rename_dist`, so it is not an exact reference
  either. EIM's `--cleanup` removes all archives after installation because
  "installation artifacts are not needed after installation"
  ([cli_commands.md#L76](https://github.com/espressif/idf-im-ui/blob/5cf0db3dcd85c66745b813b1cf31c0fe630a4ed5/docs/src/cli_commands.md?plain=1#L76)).
- EIM hardcodes the Windows tools directory `C:\Espressif\tools`, downloads
  into `C:\Espressif\dist`, and records installations in `eim_idf.json` there
  ([settings.rs#L79-L99](https://github.com/espressif/idf-im-ui/blob/5cf0db3dcd85c66745b813b1cf31c0fe630a4ed5/src-tauri/src/lib/settings.rs#L79-L99),
  [idf_config.rs#L96](https://github.com/espressif/idf-im-ui/blob/5cf0db3dcd85c66745b813b1cf31c0fe630a4ed5/src-tauri/src/lib/idf_config.rs#L96)).
  Its GUI and CLI are one executable
  ([faq.md#L277](https://github.com/espressif/idf-im-ui/blob/5cf0db3dcd85c66745b813b1cf31c0fe630a4ed5/docs/src/faq.md?plain=1#L277)).

## Foal rule

Roots, each `<tools dir>\dist`:

- non-blank absolute `IDF_TOOLS_PATH` and `%USERPROFILE%\.espressif`, only when
  the tools directory carries an ESP-IDF marker (a real `tools` directory or an
  ordinary `idf-env.json`), so a misconfigured variable never exposes unrelated
  archives;
- `%SystemDrive%\Espressif`, only when `tools\eim_idf.json` exists. This equals
  EIM's `C:\Espressif` on standard installs; a non-C: system drive yields no
  root (missed candidates, never extra ones).

Candidates are direct ordinary files with an `unpack()` suffix whose latest
modification is at least 24 hours old; recent, future, or unreadable timestamps
are skipped, and `.tmp` partial downloads never match. Re-discovery, including
the quiet window, runs again immediately before removal.

Deleting all quiet archives, rather than mirroring `--remove-archives`, is
acceptable as an explicit opt-in with disclosure: the notice states that
archives of the installed tool versions and archives staged with
`idf_tools.py download` are deleted too, so repairs, reinstalls, and offline
installs need network access.

Gate: EIM (`eim.exe`) must be idle before discovery and after measurement.
`idf.py` and `idf_tools.py` run under a shared Python runtime Foal cannot
attribute; the quiet window protects fresh downloads, but not an older archive
an install is reusing at that moment, since reuse does not touch its timestamp.
