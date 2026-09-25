# Superseded display driver packages in the Windows driver store

Status: researched 2026-09-26. This note evaluates removing third-party
display-class driver packages that no device uses and that have been superseded
by a newer package in the same family. It uses Microsoft Learn documentation and
read-only observation of one research host. It supports ADR 0036.

## Decision

| Proposed scope | Evidence result | Maximum defensible action |
| --- | --- | --- |
| Delete files under `System32\DriverStore\FileRepository` directly | **Rejected** (Microsoft warns of unpredictable behavior) | none |
| `pnputil /delete-driver ... /force`, `/uninstall`, `DiUninstallDriver` | **Rejected** (removes or replaces drivers in use) | none |
| Superseded, unused third-party Display-class packages removed through `SetupUninstallOEMInfW(name, 0, NULL)` | **Documented system removal path** | `invoke_windows_servicing` with elevation, exact selection, `--allow-servicing` |

Foal never deletes driver-store files. It asks the Windows device-installation
stack to remove a package by its published name and relies on the documented
refusal to remove a package that any device (present or not) was installed with.

## Microsoft documentation

### Removing a driver package is a system operation, not file deletion

[How Devices and Driver Packages are Uninstalled](https://learn.microsoft.com/en-us/windows-hardware/drivers/install/how-devices-and-driver-packages-are-uninstalled):

> The process of deleting a driver package from the driver store involves two
> tasks: 1. Ensure no devices are installed with the driver package. 2. Remove
> the driver package from the driver store.

> The delete process removes associated metadata from the PnP manager's internal
> database and deletes related INF files (*.inf*) from the system INF directory.

> After the driver package is removed, the package is no longer available for
> installation on a device. If you want to reinstall the package, download the
> driver package again from the original source, such as Windows Update.

> Caution: If you manually delete the driver package from the driver store, the
> system might display unpredictable behavior.

### `SetupUninstallOEMInfW` without `SUOI_FORCEDELETE` refuses in-use packages

[SetupUninstallOEMInfW](https://learn.microsoft.com/en-us/windows/win32/api/setupapi/nf-setupapi-setupuninstalloeminfw):

> The **SetupUninstallOEMInf** function uninstalls a specified .inf file and any
> associated .pnf file. If the .inf file was installed with a catalog for signing
> drivers, the catalog is also removed. A caller of this function must have
> administrative privileges, otherwise the function fails.

> **SUOI_FORCEDELETE** ... The **SetupUninstallOEMInf** function first checks
> whether there are any devices installed using the .inf file. A device does not
> need to be present to be detected as using the .inf file. If this flag is not
> set and the function finds a currently installed device that was installed
> using this .inf file, the .inf file is not removed.

Foal always passes `Flags = 0`. The in-use refusal surfaces as
`ERROR_INF_IN_USE_BY_DEVICES` and is recorded per package as `in_use`.

### Tools that change device drivers are out of scope

- [PnPUtil](https://learn.microsoft.com/en-us/windows-hardware/drivers/devtest/pnputil-command-syntax):
  `/delete-driver <oem#.inf> [/uninstall] [/force]` — `/uninstall` uninstalls the
  package from devices using it and `/force` deletes it even when in use. Foal
  never launches PnPUtil.
- [DiUninstallDriverW](https://learn.microsoft.com/en-us/windows/win32/api/newdev/nf-newdev-diuninstalldriverw)
  removes a package "from any devices it is installed on by installing those
  devices with another matching driver package ... or the null driver", with the
  caution that this "can result in replacing a more compatible or newer driver
  package with a less compatible or older driver." Foal never calls it.

### Which package in a family is newest

[INF DriverVer Directive](https://learn.microsoft.com/en-us/windows-hardware/drivers/install/inf-driverver-directive):

> When the operating system searches for drivers, it selects a driver that has a
> more recent **DriverVer** date over a driver that has an earlier date.

`DriverVer=mm/dd/yyyy,w.x.y.z` (a hyphen may separate the date fields; each
version part is below 65535). Foal orders a family by date, then version, then
published-name number, and fails closed for the whole family when any member's
`DriverVer` cannot be parsed.

## Eligibility rules

A package is a candidate only when all of the following hold (anything uncertain
yields no candidate):

1. Its published name matches `oem<digits>.inf` (a third-party package; inbox
   packages are never considered).
2. Its `[Version]` `ClassGuid` is the Display class
   `{4d36e968-e325-11ce-bfc1-08002be10318}`.
3. It belongs to a family (same original INF name, provider, and class GUID)
   with at least one newer package.
4. It is not the newest package in its family.
5. No device node — present or non-present — reports it as
   `DEVPKEY_Device_DriverInfPath`. Device enumeration failure fails the whole
   analysis.
6. Its driver-store directory is a real, non-reparse directory directly under
   `%SystemRoot%\System32\DriverStore\FileRepository` (measurement only; Foal
   never touches it).

The elevated helper repeats the whole inventory and policy and removes only the
intersection of its fresh candidate set and the requested names.

## Research-host observation (2026-09-25)

NVIDIA GeForce RTX 2070; 22 `nv_dispi.inf` packages (`oem84` … `oem213`,
2024-10 through 2026-09), each ~1.9–2.7 GB, ~48.7 GB in total. The device used
`oem213.inf` (32.0.16.1714). Under the rules above the 21 older packages
(~46 GB) are candidates. A second family (`gamevieweridddriver.inf`, a virtual
display adapter) had an unused older package and an in-use newer one.

## User impact

- Removed versions can no longer be selected by Device Manager "Roll Back
  Driver"; reinstalling one requires downloading it again.
- Removal needs administrator consent (UAC); it is not cancelable once the
  helper starts.
- Size is measured logical bytes of each package directory; the approximate
  free-space change is observed around the removal and never counted as Clean
  deleted bytes.
