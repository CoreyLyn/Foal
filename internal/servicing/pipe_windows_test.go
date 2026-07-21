//go:build windows

package servicing

import (
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// TestHelperLaunchParametersSurviveWindowsArgvParsing proves the elevated-helper
// ShellExecute parameter string is quoted with Windows rules so CommandLineToArgvW
// (and therefore os.Args in the helper) recovers the exact pipe name and nonce.
// Go's %q must not be used: it doubles backslashes and corrupts \\.\pipe\ paths.
func TestHelperLaunchParametersSurviveWindowsArgvParsing(t *testing.T) {
	pipeName := `\\.\pipe\foal-servicing-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef`
	nonce := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	params := helperLaunchParameters(pipeName, nonce)
	cmdLine := fmt.Sprintf(`"C:\Program Files\Foal\foal.exe" %s`, params)

	argv := commandLineToArgv(t, cmdLine)
	if len(argv) != 5 {
		t.Fatalf("argc = %d, want 5; argv=%q params=%q", len(argv), argv, params)
	}
	if argv[1] != HelperModeArgument {
		t.Fatalf("mode = %q, want %q", argv[1], HelperModeArgument)
	}
	if argv[2] != pipeName {
		t.Fatalf("pipe name = %q, want %q\nparams=%q", argv[2], pipeName, params)
	}
	if argv[3] != nonce {
		t.Fatalf("nonce = %q, want %q", argv[3], nonce)
	}
	if argv[4] != fmt.Sprintf("%d", protocolVersion) {
		t.Fatalf("version = %q, want %d", argv[4], protocolVersion)
	}
}

// TestWindowsQuoteArgDoesNotDoubleBackslashes is the regression lock for the
// %q bug that made AnalyzeComponentStore always fail after UAC consent.
func TestWindowsQuoteArgDoesNotDoubleBackslashes(t *testing.T) {
	pipe := `\\.\pipe\foal-servicing-abc`
	got := windowsQuoteArg(pipe)
	// Quoted form must keep a single backslash-pair prefix, not Go %q doubling.
	want := `"` + pipe + `"`
	if got != want {
		t.Fatalf("windowsQuoteArg = %q, want %q", got, want)
	}
	// %q would produce this broken form — keep the contrast explicit.
	broken := fmt.Sprintf("%q", pipe)
	if got == broken {
		t.Fatalf("windowsQuoteArg must not match Go %%q (%q)", broken)
	}
	argv := commandLineToArgv(t, "foal.exe "+got)
	if len(argv) != 2 || argv[1] != pipe {
		t.Fatalf("parsed = %q, want pipe %q", argv, pipe)
	}
}

func commandLineToArgv(t *testing.T, cmdLine string) []string {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(cmdLine)
	if err != nil {
		t.Fatalf("UTF16PtrFromString: %v", err)
	}
	var argc int32
	argvPtr, err := syscall.CommandLineToArgv(p, &argc)
	if err != nil {
		t.Fatalf("CommandLineToArgv: %v", err)
	}
	defer syscall.LocalFree((syscall.Handle)(unsafe.Pointer(argvPtr)))
	out := make([]string, 0, argc)
	for i := int32(0); i < argc; i++ {
		arg := (*[1 << 16]*uint16)(unsafe.Pointer(argvPtr))[i]
		out = append(out, syscall.UTF16ToString((*[1 << 16]uint16)(unsafe.Pointer(arg))[:]))
	}
	return out
}

// TestNamedPipeRoundTripAuthenticatesAndExchanges drives the real Windows named
// pipe end to end in one process: the coordinator creates the ACL-restricted
// pipe, a same-process "helper" connects, both validate the peer executable
// (self), and exactly one analysis request/response is exchanged. No UAC and no
// DISM are involved — analysis is injected.
func TestNamedPipeRoundTripAuthenticatesAndExchanges(t *testing.T) {
	name, err := newPipeName()
	if err != nil {
		t.Fatalf("newPipeName: %v", err)
	}
	sddl, err := servicingPipeSDDL()
	if err != nil {
		t.Fatalf("servicingPipeSDDL: %v", err)
	}
	server, err := createServerPipe(name, sddl)
	if err != nil {
		t.Fatalf("createServerPipe: %v", err)
	}
	serverConn := &pipeConn{h: server}
	defer serverConn.Close()

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	const nonce = "round-trip-nonce"

	helperDone := make(chan error, 1)
	analyzed := make(chan struct{}, 1)
	go func() {
		client, cerr := connectClientPipe(name, 10*time.Second)
		if cerr != nil {
			helperDone <- cerr
			return
		}
		clientConn := &pipeConn{h: client}
		defer clientConn.Close()
		// Validate the server peer is this same executable.
		serverPID, perr := serverProcessID(client)
		if perr != nil {
			helperDone <- perr
			return
		}
		if verr := validatePeerExecutable(serverPID, self); verr != nil {
			helperDone <- verr
			return
		}
		helperDone <- helperExchange(clientConn, nonce, analyzeDispatch(func() clean.ServicingAnalysisResult {
			analyzed <- struct{}{}
			return readyAnalyze()
		}))
	}()

	if err := connectServerPipe(server, 10*time.Second); err != nil {
		t.Fatalf("connectServerPipe: %v", err)
	}
	clientPID, err := clientProcessID(server)
	if err != nil {
		t.Fatalf("clientProcessID: %v", err)
	}
	if clientPID != uint32(windows.GetCurrentProcessId()) {
		t.Fatalf("client PID = %d, want current process %d", clientPID, windows.GetCurrentProcessId())
	}
	if err := validatePeerExecutable(clientPID, self); err != nil {
		t.Fatalf("client peer validation failed: %v", err)
	}

	resp, err := serverExchange(serverConn, nonce, wireCapabilityAnalyzeComponentStore)
	if err != nil {
		t.Fatalf("serverExchange: %v", err)
	}
	select {
	case err := <-helperDone:
		if err != nil {
			t.Fatalf("helper exchange: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("helper did not finish")
	}
	select {
	case <-analyzed:
	default:
		t.Fatal("analysis was not invoked")
	}

	res := resultFromResponse(resp)
	if res.Outcome != clean.ServicingOutcomeReady || res.ReclaimablePackages != 4 || !res.CleanupRecommended {
		t.Fatalf("round-trip result = %#v", res)
	}
}

// TestNamedPipeRoundTripRejectsNonceMismatch proves the helper over a real pipe
// refuses a request whose nonce does not match its launch nonce and never runs
// analysis.
func TestNamedPipeRoundTripRejectsNonceMismatch(t *testing.T) {
	name, err := newPipeName()
	if err != nil {
		t.Fatalf("newPipeName: %v", err)
	}
	sddl, err := servicingPipeSDDL()
	if err != nil {
		t.Fatalf("servicingPipeSDDL: %v", err)
	}
	server, err := createServerPipe(name, sddl)
	if err != nil {
		t.Fatalf("createServerPipe: %v", err)
	}
	serverConn := &pipeConn{h: server}
	defer serverConn.Close()

	analyzed := false
	helperDone := make(chan error, 1)
	go func() {
		client, cerr := connectClientPipe(name, 10*time.Second)
		if cerr != nil {
			helperDone <- cerr
			return
		}
		clientConn := &pipeConn{h: client}
		defer clientConn.Close()
		helperDone <- helperExchange(clientConn, "helper-launch-nonce", analyzeDispatch(func() clean.ServicingAnalysisResult {
			analyzed = true
			return readyAnalyze()
		}))
	}()

	if err := connectServerPipe(server, 10*time.Second); err != nil {
		t.Fatalf("connectServerPipe: %v", err)
	}
	// Coordinator sends a different nonce.
	_, _ = serverExchange(serverConn, "coordinator-nonce", wireCapabilityAnalyzeComponentStore)

	select {
	case err := <-helperDone:
		if err == nil {
			t.Fatal("helper accepted a mismatched nonce over the pipe")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("helper did not finish")
	}
	if analyzed {
		t.Fatal("helper ran analysis despite nonce mismatch")
	}
}
