//go:build windows

package driverstore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modsetupapi                         = windows.NewLazySystemDLL("setupapi.dll")
	procSetupGetInfDriverStoreLocationW = modsetupapi.NewProc("SetupGetInfDriverStoreLocationW")
	procSetupOpenInfFileW               = modsetupapi.NewProc("SetupOpenInfFileW")
	procSetupCloseInfFile               = modsetupapi.NewProc("SetupCloseInfFile")
	procSetupFindFirstLineW             = modsetupapi.NewProc("SetupFindFirstLineW")
	procSetupGetStringFieldW            = modsetupapi.NewProc("SetupGetStringFieldW")
	procSetupUninstallOEMInfW           = modsetupapi.NewProc("SetupUninstallOEMInfW")
)

// infStyleWin4 selects Windows NT-style INF parsing for SetupOpenInfFileW.
const infStyleWin4 = 0x00000002

// infContext mirrors SetupAPI INFCONTEXT.
type infContext struct {
	Inf        uintptr
	CurrentInf uintptr
	Section    uint32
	Line       uint32
}

// devpkeyDeviceDriverInfPath is DEVPKEY_Device_DriverInfPath: the published
// INF name (for example oem42.inf) a device node was installed with.
var devpkeyDeviceDriverInfPath = windows.DEVPROPKEY{
	FmtID: windows.DEVPROPGUID{
		Data1: 0xa8b865dd, Data2: 0x2e3d, Data3: 0x4094,
		Data4: [8]byte{0xad, 0x97, 0xe5, 0x93, 0xa7, 0x0c, 0x75, 0xd6},
	},
	PID: 5,
}

type versionSection struct {
	classGUID     string
	provider      string
	driverDate    string
	driverVersion string
}

// InspectDisplayPackages returns a read-only inventory of third-party
// Display-class driver packages with their measured driver-store size and
// in-use state. It needs no administrator rights and never mutates anything.
// Any failure to enumerate devices, read an oem INF, or resolve a Display
// package's driver-store directory fails the whole inventory closed.
func InspectDisplayPackages(ctx context.Context) (Inventory, error) {
	windowsDir, err := windows.GetWindowsDirectory()
	if err != nil {
		return Inventory{}, fmt.Errorf("driverstore: windows directory: %w", err)
	}
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return Inventory{}, fmt.Errorf("driverstore: system directory: %w", err)
	}
	repository := filepath.Join(systemDir, "DriverStore", "FileRepository")

	inUse, err := driverInfNamesInUse()
	if err != nil {
		return Inventory{}, err
	}

	infDir := filepath.Join(windowsDir, "INF")
	entries, err := os.ReadDir(infDir)
	if err != nil {
		return Inventory{}, fmt.Errorf("driverstore: read INF directory: %w", err)
	}
	var inventory Inventory
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return Inventory{}, err
		}
		name := entry.Name()
		if entry.IsDir() || !ValidPublishedName(name) {
			continue
		}
		infPath := filepath.Join(infDir, name)
		version, err := readVersionSection(infPath)
		if err != nil {
			return Inventory{}, fmt.Errorf("driverstore: read %s: %w", name, err)
		}
		if !strings.EqualFold(version.classGUID, DisplayClassGUID) {
			continue
		}
		storeINF, err := driverStoreLocation(infPath)
		if err != nil {
			return Inventory{}, fmt.Errorf("driverstore: locate %s: %w", name, err)
		}
		storeDir := filepath.Dir(storeINF)
		if !strings.EqualFold(filepath.Dir(storeDir), repository) {
			return Inventory{}, fmt.Errorf("driverstore: %s resolves outside the driver repository", name)
		}
		bytes, err := measurePackageDir(ctx, storeDir)
		if err != nil {
			return Inventory{}, fmt.Errorf("driverstore: measure %s: %w", name, err)
		}
		inventory.Packages = append(inventory.Packages, Package{
			PublishedName: name,
			OriginalName:  filepath.Base(storeINF),
			Provider:      version.provider,
			ClassGUID:     version.classGUID,
			DriverDate:    version.driverDate,
			DriverVersion: version.driverVersion,
			Bytes:         bytes,
			InUse:         inUse[strings.ToLower(name)],
		})
	}
	return inventory, nil
}

// RemovePackage asks Windows to remove one published driver package without
// SUOI_FORCEDELETE. Windows refuses packages any device was installed with,
// reported as in_use. Requires administrator rights. Invalid names are never
// passed to the API.
func RemovePackage(name string) RemoveOutcome {
	if !ValidPublishedName(name) {
		return RemoveOutcomeFailed
	}
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return RemoveOutcomeFailed
	}
	ok, _, callErr := procSetupUninstallOEMInfW.Call(uintptr(unsafe.Pointer(ptr)), 0, 0)
	if ok != 0 {
		return RemoveOutcomeRemoved
	}
	if errors.Is(callErr, windows.ERROR_INF_IN_USE_BY_DEVICES) {
		return RemoveOutcomeInUse
	}
	return RemoveOutcomeFailed
}

// driverInfNamesInUse enumerates every device node of every class, present or
// not, and collects its DEVPKEY_Device_DriverInfPath.
func driverInfNamesInUse() (map[string]bool, error) {
	set, err := windows.SetupDiGetClassDevsEx(nil, "", 0, windows.DIGCF_ALLCLASSES, 0, "")
	if err != nil {
		return nil, fmt.Errorf("driverstore: enumerate devices: %w", err)
	}
	defer windows.SetupDiDestroyDeviceInfoList(set)
	used := map[string]bool{}
	for index := 0; ; index++ {
		data, err := windows.SetupDiEnumDeviceInfo(set, index)
		if err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
				return used, nil
			}
			return nil, fmt.Errorf("driverstore: enumerate device %d: %w", index, err)
		}
		value, err := windows.SetupDiGetDeviceProperty(set, data, &devpkeyDeviceDriverInfPath)
		if err != nil {
			if errors.Is(err, windows.ERROR_NOT_FOUND) {
				continue
			}
			return nil, fmt.Errorf("driverstore: device %d driver INF: %w", index, err)
		}
		if name, ok := value.(string); ok && name != "" {
			used[strings.ToLower(filepath.Base(name))] = true
		}
	}
}

func readVersionSection(path string) (versionSection, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return versionSection{}, err
	}
	var errorLine uint32
	handle, _, callErr := procSetupOpenInfFileW.Call(uintptr(unsafe.Pointer(pathPtr)), 0, infStyleWin4, uintptr(unsafe.Pointer(&errorLine)))
	if handle == 0 || handle == uintptr(windows.InvalidHandle) {
		return versionSection{}, callErr
	}
	defer procSetupCloseInfFile.Call(handle)

	var section versionSection
	section.classGUID, _ = infField(handle, "Version", "ClassGuid", 1)
	section.provider, _ = infField(handle, "Version", "Provider", 1)
	section.driverDate, _ = infField(handle, "Version", "DriverVer", 1)
	section.driverVersion, _ = infField(handle, "Version", "DriverVer", 2)
	section.classGUID = strings.TrimSpace(section.classGUID)
	return section, nil
}

// infField reads one field of the first matching line; SetupAPI performs
// %strkey% substitution from the INF [Strings] sections.
func infField(handle uintptr, section, key string, field uint32) (string, bool) {
	sectionPtr, err := windows.UTF16PtrFromString(section)
	if err != nil {
		return "", false
	}
	keyPtr, err := windows.UTF16PtrFromString(key)
	if err != nil {
		return "", false
	}
	var context infContext
	found, _, _ := procSetupFindFirstLineW.Call(handle, uintptr(unsafe.Pointer(sectionPtr)), uintptr(unsafe.Pointer(keyPtr)), uintptr(unsafe.Pointer(&context)))
	if found == 0 {
		return "", false
	}
	size := uint32(256)
	for attempt := 0; attempt < 2; attempt++ {
		buffer := make([]uint16, size)
		var required uint32
		ok, _, _ := procSetupGetStringFieldW.Call(uintptr(unsafe.Pointer(&context)), uintptr(field),
			uintptr(unsafe.Pointer(&buffer[0])), uintptr(size), uintptr(unsafe.Pointer(&required)))
		if ok != 0 {
			return windows.UTF16ToString(buffer), true
		}
		if required <= size {
			return "", false
		}
		size = required
	}
	return "", false
}

func driverStoreLocation(infPath string) (string, error) {
	pathPtr, err := windows.UTF16PtrFromString(infPath)
	if err != nil {
		return "", err
	}
	size := uint32(windows.MAX_PATH)
	for attempt := 0; attempt < 2; attempt++ {
		buffer := make([]uint16, size)
		var required uint32
		ok, _, callErr := procSetupGetInfDriverStoreLocationW.Call(uintptr(unsafe.Pointer(pathPtr)), 0, 0,
			uintptr(unsafe.Pointer(&buffer[0])), uintptr(size), uintptr(unsafe.Pointer(&required)))
		if ok != 0 {
			return windows.UTF16ToString(buffer), nil
		}
		if required <= size {
			return "", callErr
		}
		size = required
	}
	return "", errors.New("driver store location buffer did not converge")
}

// measurePackageDir sums logical file sizes under a real, non-reparse package
// directory. Nested reparse points are not followed.
func measurePackageDir(ctx context.Context, dir string) (int64, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || isReparse(info) {
		return 0, errors.New("package directory is not a real directory")
	}
	var total int64
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || isReparse(info) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func isReparse(info os.FileInfo) bool {
	data, ok := info.Sys().(*windows.Win32FileAttributeData)
	return ok && data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
