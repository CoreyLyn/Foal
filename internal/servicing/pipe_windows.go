//go:build windows

package servicing

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var errPipeTimeout = errors.New("servicing: named pipe establishment timed out")

// pipeConn adapts a Windows named-pipe handle (opened for overlapped I/O) to an
// io.ReadWriteCloser so the shared, cross-platform one-request exchange can run
// over the real pipe. Reads and writes wait indefinitely: DISM has no forced
// wall-clock timeout, so the coordinator waits for the helper's response for as
// long as the analysis runs.
type pipeConn struct {
	h windows.Handle
}

func (c *pipeConn) Read(p []byte) (int, error) {
	n, err := overlappedIO(c.h, func(ov *windows.Overlapped) error {
		return windows.ReadFile(c.h, p, nil, ov)
	})
	if err != nil {
		return int(n), err
	}
	if n == 0 {
		return 0, io.EOF
	}
	return int(n), nil
}

func (c *pipeConn) Write(p []byte) (int, error) {
	n, err := overlappedIO(c.h, func(ov *windows.Overlapped) error {
		return windows.WriteFile(c.h, p, nil, ov)
	})
	return int(n), err
}

func (c *pipeConn) Close() error { return windows.CloseHandle(c.h) }

// overlappedIO issues one overlapped operation and waits for it to complete with
// no wall-clock timeout, returning the number of bytes transferred.
func overlappedIO(h windows.Handle, op func(*windows.Overlapped) error) (uint32, error) {
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(event)
	ov := &windows.Overlapped{HEvent: event}
	err = op(ov)
	if err != nil && err != windows.ERROR_IO_PENDING {
		return 0, err
	}
	if err == windows.ERROR_IO_PENDING {
		if _, werr := windows.WaitForSingleObject(event, windows.INFINITE); werr != nil {
			return 0, werr
		}
	}
	var transferred uint32
	if err := windows.GetOverlappedResult(h, ov, &transferred, false); err != nil {
		return transferred, err
	}
	return transferred, nil
}

// newPipeName returns a fresh, randomized local named-pipe path. The randomness
// keeps a foreign process from predicting the name; the ACL and nonce provide
// the actual authorization and replay protection.
func newPipeName() (string, error) {
	suffix, err := newNonce()
	if err != nil {
		return "", err
	}
	return `\\.\pipe\foal-servicing-` + suffix, nil
}

// servicingPipeSDDL builds a protected DACL granting full control only to the
// invoking user, the local Administrators group, and LocalSystem. No other
// principal may open the pipe, and inheritance is disabled.
func servicingPipeSDDL() (string, error) {
	sid, err := currentUserSID()
	if err != nil {
		return "", err
	}
	// P = protected (no inheritance); GA = generic all. SY = LocalSystem,
	// BA = BUILTIN\Administrators.
	return fmt.Sprintf("D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;%s)", sid), nil
}

func currentUserSID() (string, error) {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

// createServerPipe creates the single-instance, overlapped, byte-mode named pipe
// with the given SDDL and rejects remote clients.
func createServerPipe(name, sddl string) (windows.Handle, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return windows.InvalidHandle, err
	}
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return windows.InvalidHandle, err
	}
	sa := windows.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = uint32(unsafe.Sizeof(sa))
	handle, err := windows.CreateNamedPipe(
		namePtr,
		windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS,
		1, // exactly one instance: no second coordinator can attach
		4096, 4096,
		0,
		&sa,
	)
	if err != nil {
		return windows.InvalidHandle, err
	}
	return handle, nil
}

// connectServerPipe waits for the helper to connect, bounded by timeout. It
// never bounds the later response read, which awaits DISM.
func connectServerPipe(pipe windows.Handle, timeout time.Duration) error {
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(event)
	ov := &windows.Overlapped{HEvent: event}
	err = windows.ConnectNamedPipe(pipe, ov)
	if err == nil || err == windows.ERROR_PIPE_CONNECTED {
		return nil
	}
	if err != windows.ERROR_IO_PENDING {
		return err
	}
	state, werr := windows.WaitForSingleObject(event, uint32(timeout/time.Millisecond))
	if werr != nil {
		return werr
	}
	if state != windows.WAIT_OBJECT_0 {
		_ = windows.CancelIoEx(pipe, ov)
		return errPipeTimeout
	}
	var transferred uint32
	return windows.GetOverlappedResult(pipe, ov, &transferred, false)
}

// connectClientPipe opens the coordinator's pipe for overlapped duplex I/O,
// retrying briefly while the single instance is momentarily busy.
func connectClientPipe(name string, timeout time.Duration) (windows.Handle, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return windows.InvalidHandle, err
	}
	deadline := time.Now().Add(timeout)
	for {
		handle, err := windows.CreateFile(
			namePtr,
			windows.GENERIC_READ|windows.GENERIC_WRITE,
			0,
			nil,
			windows.OPEN_EXISTING,
			windows.FILE_FLAG_OVERLAPPED,
			0,
		)
		if err == nil {
			return handle, nil
		}
		if err != windows.ERROR_PIPE_BUSY || time.Now().After(deadline) {
			return windows.InvalidHandle, err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// clientProcessID returns the PID of the process connected as the pipe client.
func clientProcessID(pipe windows.Handle) (uint32, error) {
	var pid uint32
	if err := windows.GetNamedPipeClientProcessId(pipe, &pid); err != nil {
		return 0, err
	}
	return pid, nil
}

// serverProcessID returns the PID of the process that created the pipe (server).
func serverProcessID(pipe windows.Handle) (uint32, error) {
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(pipe, &pid); err != nil {
		return 0, err
	}
	return pid, nil
}

// validatePeerExecutable confirms the peer process is running the same Foal
// executable as this process, so a foreign process cannot impersonate the
// coordinator or the helper even if it reaches the pipe.
func validatePeerExecutable(pid uint32, ownExecutable string) error {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return err
	}
	peer := windows.UTF16ToString(buf[:size])
	if !sameExecutable(peer, ownExecutable) {
		return fmt.Errorf("servicing: peer executable %q is not this Foal binary", peer)
	}
	return nil
}

func sameExecutable(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// acquireServicingMutex takes a global named mutex so only one Foal servicing
// helper coordinates at a time. It does not infer Windows servicing idleness
// from any process; CBS/DISM owns system transaction serialization. The returned
// release function must be called when coordination finishes.
func acquireServicingMutex() (func(), bool) {
	namePtr, err := windows.UTF16PtrFromString(`Global\FoalServicingCoordinator`)
	if err != nil {
		return nil, false
	}
	handle, err := windows.CreateMutex(nil, false, namePtr)
	if err != nil && handle == 0 {
		return nil, false
	}
	state, werr := windows.WaitForSingleObject(handle, 0)
	if werr != nil || (state != windows.WAIT_OBJECT_0 && state != windows.WAIT_ABANDONED) {
		windows.CloseHandle(handle)
		return nil, false
	}
	return func() {
		_ = windows.ReleaseMutex(handle)
		_ = windows.CloseHandle(handle)
	}, true
}

// launchElevatedHelper starts an elevated copy of this Foal executable in the
// internal helper mode via ShellExecute "runas", passing only the pipe name,
// nonce, and protocol version — never the capability, a DISM argument, or a
// path. A user declining the UAC prompt is reported distinctly from a launch
// failure.
func launchElevatedHelper(executable, pipeName, nonce string) error {
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return err
	}
	// Use Windows command-line quoting, not Go %q: %q doubles backslashes for
	// Go string syntax, but CommandLineToArgvW treats backslashes as literals
	// (except before quotes). Doubled backslashes corrupt \\.\pipe\... so the
	// elevated helper can never open the coordinator pipe.
	params, err := windows.UTF16PtrFromString(helperLaunchParameters(pipeName, nonce))
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, params, nil, windows.SW_HIDE)
}

// helperLaunchParameters builds the ShellExecute lpParameters string for the
// elevated helper. Pipe names and nonces are quoted with Windows rules so
// CommandLineToArgvW (and Go's os.Args) recovers the exact values.
func helperLaunchParameters(pipeName, nonce string) string {
	return fmt.Sprintf("%s %s %s %d",
		HelperModeArgument,
		windowsQuoteArg(pipeName),
		windowsQuoteArg(nonce),
		protocolVersion,
	)
}

// windowsQuoteArg quotes s for a Windows process command line so
// CommandLineToArgvW yields the original string. Backslashes are left intact
// (unlike Go %q); only embedded double quotes are escaped by doubling.
func windowsQuoteArg(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\"") {
		// No whitespace or quotes: still quote pipe paths so a future format
		// change cannot split \\.\pipe\... and so the rule is uniform for the
		// helper argument vector.
		return `"` + s + `"`
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			b.WriteByte('"')
		}
		b.WriteByte(s[i])
	}
	b.WriteByte('"')
	return b.String()
}

// isUserDeclinedElevation reports whether a launch error is the user declining
// the UAC prompt rather than a genuine launch failure.
func isUserDeclinedElevation(err error) bool {
	return errors.Is(err, windows.ERROR_CANCELLED) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
