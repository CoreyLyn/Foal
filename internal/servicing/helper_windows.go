//go:build windows

package servicing

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// RunHelper is the internal, elevated servicing helper entry point. It is
// reachable only through HelperModeArgument and does nothing until it has
// authenticated its launching coordinator over the nonce-bound pipe: it
// validates the server peer is the same Foal executable, reads exactly one
// versioned, nonce-matched request naming a fixed built-in capability, runs the
// fixed DISM analysis, and returns exactly one path-free response.
//
// args is os.Args[1:], i.e. [HelperModeArgument, pipeName, nonce, protocolVersion].
func RunHelper(args []string) int {
	if len(args) < 4 || args[0] != HelperModeArgument {
		return helperExitError
	}
	pipeName := args[1]
	launchNonce := args[2]
	launchVersion, err := strconv.ParseUint(args[3], 10, 32)
	if err != nil || uint32(launchVersion) != protocolVersion {
		return helperExitError
	}

	executable, err := os.Executable()
	if err != nil {
		return helperExitError
	}
	if abs, aerr := filepath.Abs(executable); aerr == nil {
		executable = abs
	}

	pipe, err := connectClientPipe(pipeName, helperEstablishTimeout)
	if err != nil {
		return helperExitError
	}
	conn := &pipeConn{h: pipe}
	defer conn.Close()

	// Authenticate the server peer is the same Foal executable before accepting
	// any data.
	serverPID, err := serverProcessID(pipe)
	if err != nil {
		return helperExitError
	}
	if err := validatePeerExecutable(serverPID, executable); err != nil {
		return helperExitError
	}

	if err := helperExchange(conn, launchNonce, dispatchCapability); err != nil {
		return helperExitError
	}
	return helperExitSuccess
}

// dispatchCapability runs the fixed built-in capability requested over the pipe.
// Only the two known capabilities exist; validateRequest already rejected any
// other value, so the default is defensive and runs no DISM.
func dispatchCapability(capability wireCapability) pipeResponse {
	switch capability {
	case wireCapabilityAnalyzeComponentStore:
		return responseFromAnalysis(runComponentStoreAnalysis())
	case wireCapabilityExecuteComponentStoreCleanup:
		return responseFromExecute(runComponentStoreCleanup())
	default:
		return responseFromExecute(failExecuteResult(clean.ServicingReasonHelperFailed))
	}
}
