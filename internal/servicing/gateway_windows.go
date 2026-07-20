//go:build windows

package servicing

import (
	"context"
	"os"
	"path/filepath"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// windowsGateway coordinates component-store analysis and the composite cleanup
// through an isolated, elevated helper of the same Foal executable. The
// coordinator itself stays non-elevated: it only creates the ACL-restricted,
// nonce-bound pipe, launches the helper with UAC, authenticates the peer, and
// relays a fixed built-in capability. All file deletion stays in the
// non-elevated coordinator.
type windowsGateway struct{}

// NewGateway returns the production Windows servicing gateway.
func NewGateway() clean.ServicingGateway { return windowsGateway{} }

func (windowsGateway) AnalyzeComponentStore(ctx context.Context, req clean.ServicingAnalysisRequest) clean.ServicingAnalysisResult {
	// Only the read-only analysis capability is valid here. Any other request
	// fails closed rather than reaching the helper.
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

func (windowsGateway) ExecuteComponentStoreCleanup(ctx context.Context, req clean.ServicingExecuteRequest) clean.ServicingExecuteResult {
	// Only the composite execute capability is valid here. There is no standalone
	// start-cleanup capability, so cleanup can never be requested without a fresh
	// in-helper analysis.
	if req.Capability != clean.ServicingCapabilityExecuteComponentStoreCleanup {
		return failExecuteResult(clean.ServicingReasonHelperFailed)
	}
	if ctx != nil && ctx.Err() != nil {
		// Pre-mutation cancellation: never begin a servicing action. No UAC, no DISM.
		return canceledExecuteResult()
	}
	return coordinateCleanup(ctx)
}

// helperSession holds an authenticated coordinator/helper connection ready for
// exactly one request. The caller owns closing conn and calling release.
type helperSession struct {
	conn    *pipeConn
	nonce   string
	release func()
}

// establishHelper serializes Foal servicing, creates the ACL-restricted,
// nonce-bound pipe, elevates the helper, waits (bounded) for it to connect, and
// authenticates the peer executable. On any failure it releases every resource
// and returns a stable reason plus whether the failure is a skip (UAC decline)
// rather than a hard failure.
func establishHelper() (session *helperSession, reason string, isSkip bool, ok bool) {
	release, acquired := acquireServicingMutex()
	if !acquired {
		return nil, clean.ServicingReasonHelperFailed, false, false
	}
	cleanup := release
	defer func() {
		if !ok && cleanup != nil {
			cleanup()
		}
	}()

	executable, err := os.Executable()
	if err != nil {
		return nil, clean.ServicingReasonHelperFailed, false, false
	}
	if abs, aerr := filepath.Abs(executable); aerr == nil {
		executable = abs
	}

	nonce, err := newNonce()
	if err != nil {
		return nil, clean.ServicingReasonHelperFailed, false, false
	}
	pipeName, err := newPipeName()
	if err != nil {
		return nil, clean.ServicingReasonHelperFailed, false, false
	}
	sddl, err := servicingPipeSDDL()
	if err != nil {
		return nil, clean.ServicingReasonHelperFailed, false, false
	}

	pipe, err := createServerPipe(pipeName, sddl)
	if err != nil {
		return nil, clean.ServicingReasonHelperFailed, false, false
	}
	conn := &pipeConn{h: pipe}
	closePipe := true
	defer func() {
		if closePipe {
			conn.Close()
		}
	}()

	// Elevate a copy of this same executable in helper mode. UAC decline is a
	// stable skip; any other launch failure is an elevation failure.
	if err := launchElevatedHelper(executable, pipeName, nonce); err != nil {
		if isUserDeclinedElevation(err) {
			return nil, clean.ServicingReasonElevationDenied, true, false
		}
		return nil, clean.ServicingReasonElevationFailed, false, false
	}

	// Bound only UAC + connection establishment.
	if err := connectServerPipe(pipe, helperEstablishTimeout); err != nil {
		return nil, clean.ServicingReasonHelperFailed, false, false
	}

	// Authenticate the connected client is the same Foal executable.
	clientPID, err := clientProcessID(pipe)
	if err != nil {
		return nil, clean.ServicingReasonHelperFailed, false, false
	}
	if err := validatePeerExecutable(clientPID, executable); err != nil {
		return nil, clean.ServicingReasonHelperFailed, false, false
	}

	closePipe = false
	cleanup = nil
	return &helperSession{conn: conn, nonce: nonce, release: release}, "", false, true
}

// coordinateAnalysis performs the full non-elevated coordination for read-only
// analysis: establish the authenticated helper and relay one analysis request.
func coordinateAnalysis() clean.ServicingAnalysisResult {
	session, reason, isSkip, ok := establishHelper()
	if !ok {
		if isSkip {
			return skipResult(reason)
		}
		return failResult(reason)
	}
	defer session.release()
	defer session.conn.Close()

	resp, err := serverExchange(session.conn, session.nonce, wireCapabilityAnalyzeComponentStore)
	if err != nil {
		return failResult(clean.ServicingReasonHelperFailed)
	}
	return resultFromResponse(resp)
}

// coordinateCleanup performs the composite execute coordination. Once the
// execute request is sent, cleanup may already be underway: a cancellation is
// recorded but DISM, the helper, and TrustedInstaller are never terminated, and
// the coordinator waits for the actual exit outcome.
func coordinateCleanup(ctx context.Context) clean.ServicingExecuteResult {
	session, reason, isSkip, ok := establishHelper()
	if !ok {
		if isSkip {
			return skipExecuteResult(reason)
		}
		return failExecuteResult(reason)
	}
	defer session.release()
	defer session.conn.Close()

	// Pre-mutation cancellation window: if canceled before the execute request is
	// sent, the helper never runs DISM (it blocks on the request read, then exits
	// when the pipe closes). No StartComponentCleanup begins.
	if ctx != nil && ctx.Err() != nil {
		return canceledExecuteResult()
	}

	type exchangeOutcome struct {
		resp pipeResponse
		err  error
	}
	ch := make(chan exchangeOutcome, 1)
	go func() {
		resp, err := serverExchange(session.conn, session.nonce, wireCapabilityExecuteComponentStoreCleanup)
		ch <- exchangeOutcome{resp: resp, err: err}
	}()

	cancelRequested := false
	var out exchangeOutcome
	if done := ctxDoneChannel(ctx); done != nil {
		select {
		case out = <-ch:
		case <-done:
			// Cleanup may already have started. Record that cancellation was
			// requested but keep waiting for the actual outcome; never kill DISM,
			// the helper, or TrustedInstaller.
			cancelRequested = true
			out = <-ch
		}
	} else {
		out = <-ch
	}

	if out.err != nil {
		res := failExecuteResult(clean.ServicingReasonHelperFailed)
		res.CancelRequested = cancelRequested
		return res
	}
	res := executeResultFromResponse(out.resp)
	if cancelRequested {
		res.CancelRequested = true
	}
	return res
}

// ctxDoneChannel returns ctx.Done() or nil when ctx is nil, so a nil context
// simply waits for the servicing outcome without a cancellation branch.
func ctxDoneChannel(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return nil
	}
	return ctx.Done()
}
