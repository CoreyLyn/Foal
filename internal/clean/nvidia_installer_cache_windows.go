//go:build windows

package clean

import (
	"context"
	"errors"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// productionDetectNVIDIAActivity conservatively reports NVIDIA process/service
// activity. Any nv-named process or non-stopped NVIDIA-owned service is running;
// any enumeration failure or uncertain attribution is unknown. Idle requires
// both a clean process snapshot and a reachable, all-idle NVIDIA service set.
// Clean never stops these processes or services.
func productionDetectNVIDIAActivity(ctx context.Context) NVIDIAActivityState {
	snapshot := snapshotProcesses(ctx)
	if snapshot.Err != nil {
		return NVIDIAActivityState{Status: NVIDIAActivityUnknown, Message: "NVIDIA process snapshot was unavailable"}
	}
	for _, name := range snapshot.Names {
		if isNVIDIAProcessName(name) {
			return NVIDIAActivityState{Status: NVIDIAActivityRunning, Message: "an NVIDIA process is running"}
		}
	}
	active, err := nvidiaServicesActive()
	if err != nil {
		return NVIDIAActivityState{Status: NVIDIAActivityUnknown, Message: "NVIDIA service state could not be determined"}
	}
	if active {
		return NVIDIAActivityState{Status: NVIDIAActivityRunning, Message: "an NVIDIA service is running"}
	}
	return NVIDIAActivityState{Status: NVIDIAActivityIdle}
}

// isNVIDIAProcessName matches NVIDIA's own conflict-resolution guidance to end
// processes beginning with nv or NVIDIA. This is intentionally broad and accepts
// frequent false skips over risking an unlisted writer.
func isNVIDIAProcessName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	return strings.HasPrefix(lower, "nv") || strings.Contains(lower, "nvidia")
}

// isNVIDIAServiceName matches NVIDIA-owned service short or display names using
// the same broad nv/NVIDIA heuristic.
func isNVIDIAServiceName(serviceName, displayName string) bool {
	return isNVIDIAProcessName(serviceName) || isNVIDIAProcessName(displayName)
}

// nvidiaServicesActive reports whether any NVIDIA-owned Win32 service is in a
// non-stopped state. It opens the SCM with read-only enumeration access; any
// failure is returned as an error so the caller fails closed to unknown.
func nvidiaServicesActive() (bool, error) {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT|windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return false, err
	}
	defer windows.CloseServiceHandle(scm)

	var bytesNeeded, servicesReturned, resumeHandle uint32
	err = windows.EnumServicesStatusEx(scm, windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_WIN32, windows.SERVICE_STATE_ALL, nil, 0, &bytesNeeded, &servicesReturned, &resumeHandle, nil)
	if err != nil && err != windows.ERROR_MORE_DATA {
		return false, err
	}
	if bytesNeeded == 0 {
		return false, nil
	}
	buf := make([]byte, bytesNeeded)
	err = windows.EnumServicesStatusEx(scm, windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_WIN32, windows.SERVICE_STATE_ALL, &buf[0], uint32(len(buf)), &bytesNeeded, &servicesReturned, &resumeHandle, nil)
	if err != nil {
		return false, err
	}
	if servicesReturned == 0 {
		return false, nil
	}
	services := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0])), int(servicesReturned))
	for i := range services {
		service := services[i]
		name := utf16PtrToStringSafe(service.ServiceName)
		display := utf16PtrToStringSafe(service.DisplayName)
		if !isNVIDIAServiceName(name, display) {
			continue
		}
		if service.ServiceStatusProcess.CurrentState != windows.SERVICE_STOPPED {
			return true, nil
		}
	}
	return false, nil
}

func utf16PtrToStringSafe(p *uint16) string {
	if p == nil {
		return ""
	}
	return windows.UTF16PtrToString(p)
}

// productionVerifyNVIDIASignature verifies a valid Authenticode signature whose
// signer subject is NVIDIA Corporation. Signature-chain failure or a non-NVIDIA
// signer returns an error so the payload is excluded.
func productionVerifyNVIDIASignature(path string) error {
	path16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	data := &windows.WinTrustData{
		Size:             uint32(unsafe.Sizeof(windows.WinTrustData{})),
		UIChoice:         windows.WTD_UI_NONE,
		RevocationChecks: windows.WTD_REVOKE_NONE,
		UnionChoice:      windows.WTD_CHOICE_FILE,
		StateAction:      windows.WTD_STATEACTION_VERIFY,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&windows.WinTrustFileInfo{
			Size:     uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})),
			FilePath: path16,
		}),
	}
	verifyErr := windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	data.StateAction = windows.WTD_STATEACTION_CLOSE
	_ = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	if verifyErr != nil {
		return verifyErr
	}

	matches, err := nvidiaSignerSubjectMatches(path16)
	if err != nil {
		return err
	}
	if !matches {
		return errors.New("payload signer subject is not NVIDIA Corporation")
	}
	return nil
}

// nvidiaSignerSubjectMatches reports whether any certificate embedded in the
// file's PKCS#7 signed store has the simple display subject "NVIDIA Corporation".
// Chain CA certificates are never named "NVIDIA Corporation", so a match
// identifies the leaf signer.
func nvidiaSignerSubjectMatches(path16 *uint16) (bool, error) {
	var certStore windows.Handle
	err := windows.CryptQueryObject(
		windows.CERT_QUERY_OBJECT_FILE,
		unsafe.Pointer(path16),
		windows.CERT_QUERY_CONTENT_FLAG_PKCS7_SIGNED_EMBED,
		windows.CERT_QUERY_FORMAT_FLAG_BINARY,
		0, nil, nil, nil,
		&certStore, nil, nil,
	)
	if err != nil {
		return false, err
	}
	defer windows.CertCloseStore(certStore, 0)

	var prev *windows.CertContext
	for {
		cert, err := windows.CertEnumCertificatesInStore(certStore, prev)
		if err != nil || cert == nil {
			break
		}
		size := windows.CertGetNameString(cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, nil, 0)
		if size > 1 {
			name := make([]uint16, size)
			windows.CertGetNameString(cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, &name[0], size)
			if strings.EqualFold(strings.TrimSpace(windows.UTF16ToString(name)), "NVIDIA Corporation") {
				return true, nil
			}
		}
		prev = cert
	}
	return false, nil
}

// fileStreamInfo mirrors FILE_STREAM_INFO. StreamName follows the fixed header.
type fileStreamInfo struct {
	NextEntryOffset      uint32
	StreamNameLength     uint32
	StreamSize           int64
	StreamAllocationSize int64
}

const fileStreamInfoHeaderSize = 24

// productionInspectNVIDIAPayloadForensics returns the payload's hard-link count
// and alternate-data-stream presence. Any failure returns an error so the caller
// fails closed.
func productionInspectNVIDIAPayloadForensics(path string) (NVIDIAPayloadForensics, error) {
	path16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return NVIDIAPayloadForensics{}, err
	}
	handle, err := windows.CreateFile(
		path16,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return NVIDIAPayloadForensics{}, err
	}
	defer windows.CloseHandle(handle)

	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return NVIDIAPayloadForensics{}, err
	}
	hasADS, err := fileHasAlternateDataStreams(handle)
	if err != nil {
		return NVIDIAPayloadForensics{}, err
	}
	return NVIDIAPayloadForensics{
		HardLinkCount:           info.NumberOfLinks,
		HasAlternateDataStreams: hasADS,
	}, nil
}

// fileHasAlternateDataStreams reports whether the file carries any stream other
// than the default "::$DATA" data stream. A buffer overflow is treated
// conservatively as ADS present.
func fileHasAlternateDataStreams(handle windows.Handle) (bool, error) {
	buf := make([]byte, 64*1024)
	err := windows.GetFileInformationByHandleEx(handle, windows.FileStreamInfo, &buf[0], uint32(len(buf)))
	if err != nil {
		switch err {
		case windows.ERROR_HANDLE_EOF:
			return false, nil
		case windows.ERROR_MORE_DATA:
			return true, nil
		default:
			return false, err
		}
	}
	offset := 0
	for {
		if offset+fileStreamInfoHeaderSize > len(buf) {
			return false, nil
		}
		entry := (*fileStreamInfo)(unsafe.Pointer(&buf[offset]))
		nameStart := offset + fileStreamInfoHeaderSize
		nameEnd := nameStart + int(entry.StreamNameLength)
		if nameEnd > len(buf) {
			return true, nil
		}
		name := decodeUTF16Bytes(buf[nameStart:nameEnd])
		if !strings.EqualFold(name, "::$DATA") {
			return true, nil
		}
		if entry.NextEntryOffset == 0 {
			return false, nil
		}
		offset += int(entry.NextEntryOffset)
	}
}

func decodeUTF16Bytes(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	units := make([]uint16, len(b)/2)
	for i := range units {
		units[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return windows.UTF16ToString(units)
}
