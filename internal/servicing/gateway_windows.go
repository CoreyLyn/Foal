//go:build windows

package servicing

import (
	"context"
	"os"
	"path/filepath"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// windowsGateway coordinates read-only component-store analysis through an
// isolated, elevated helper of the same Foal executable. The coordinator itself
// stays non-elevated: it only creates the ACL-restricted, nonce-bound pipe,
// launches the helper with UAC, authenticates the peer, and relays the fixed
// analysis capability.
type windowsGateway struct{}

// NewGateway returns the production Windows servicing gateway.
func NewGateway() clean.ServicingGateway { return windowsGateway{} }

func (windowsGateway) AnalyzeComponentStore(ctx context.Context, req clean.ServicingAnalysisRequest) clean.ServicingAnalysisResult {
	// Only the read-only analysis capability exists in this slice. Any other
	// request fails closed rather than reaching the helper.
	if req.Capability != clean.ServicingCapabilityAnalyzeComponentStore {
		return failResult(clean.ServicingReasonHelperFailed)
	}
	if ctx != nil && ctx.Err() != nil {
		return clean.ServicingAnalysisResult{
			Outcome: clean.ServicingOutcomeCanceled,
			Reason:  clean.ServicingReasonContextCanceled,
		}
	}
	return coordinateAnalysis()
}

// coordinateAnalysis performs the full non-elevated coordination: serialize Foal
// servicing, create the pipe, elevate the helper, authenticate it, and relay one
// analysis request. Every failure maps to a stable, path-free reason.
func coordinateAnalysis() clean.ServicingAnalysisResult {
	release, ok := acquireServicingMutex()
	if !ok {
		return failResult(clean.ServicingReasonHelperFailed)
	}
	defer release()

	executable, err := os.Executable()
	if err != nil {
		return failResult(clean.ServicingReasonHelperFailed)
	}
	if abs, aerr := filepath.Abs(executable); aerr == nil {
		executable = abs
	}

	nonce, err := newNonce()
	if err != nil {
		return failResult(clean.ServicingReasonHelperFailed)
	}
	pipeName, err := newPipeName()
	if err != nil {
		return failResult(clean.ServicingReasonHelperFailed)
	}
	sddl, err := servicingPipeSDDL()
	if err != nil {
		return failResult(clean.ServicingReasonHelperFailed)
	}

	pipe, err := createServerPipe(pipeName, sddl)
	if err != nil {
		return failResult(clean.ServicingReasonHelperFailed)
	}
	conn := &pipeConn{h: pipe}
	defer conn.Close()

	// Elevate a copy of this same executable in helper mode. UAC decline is a
	// stable skip; any other launch failure is an elevation failure.
	if err := launchElevatedHelper(executable, pipeName, nonce); err != nil {
		if isUserDeclinedElevation(err) {
			return skipResult(clean.ServicingReasonElevationDenied)
		}
		return failResult(clean.ServicingReasonElevationFailed)
	}

	// Bound only UAC + connection establishment.
	if err := connectServerPipe(pipe, helperEstablishTimeout); err != nil {
		return failResult(clean.ServicingReasonHelperFailed)
	}

	// Authenticate the connected client is the same Foal executable.
	clientPID, err := clientProcessID(pipe)
	if err != nil {
		return failResult(clean.ServicingReasonHelperFailed)
	}
	if err := validatePeerExecutable(clientPID, executable); err != nil {
		return failResult(clean.ServicingReasonHelperFailed)
	}

	resp, err := serverExchange(conn, nonce)
	if err != nil {
		return failResult(clean.ServicingReasonHelperFailed)
	}
	return resultFromResponse(resp)
}
