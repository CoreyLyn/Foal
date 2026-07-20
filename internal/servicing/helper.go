package servicing

import (
	"io"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// HelperModeArgument is the hidden first argument that selects the internal,
// elevated servicing helper mode of the Foal executable. It is not a public
// command, has no help entry, and does nothing on its own: the helper acts only
// after authenticating its launching coordinator over the nonce-bound named
// pipe. cmd/foal routes such invocations to RunHelper before ordinary CLI
// parsing so the mode is never reachable as a user command.
const HelperModeArgument = "__servicing-helper"

// IsHelperInvocation reports whether the process arguments (os.Args[1:]) select
// the internal servicing helper mode.
func IsHelperInvocation(args []string) bool {
	return len(args) >= 1 && args[0] == HelperModeArgument
}

// Helper process exit codes. They are internal to the coordinator/helper
// contract and are never surfaced as Foal command exit codes.
const (
	helperExitSuccess     = 0
	helperExitError       = 1
	helperExitUnsupported = 2
)

// helperEstablishTimeout bounds UAC consent plus named-pipe connection
// establishment (ADR 0029). It never bounds DISM, which has no forced
// wall-clock timeout.
const helperEstablishTimeout = 2 * time.Minute

// serverExchange is the coordinator side of the one-request protocol. It sends
// exactly one request for the given capability bound to nonce and reads exactly
// one response, validating the protocol version. It performs no second request.
func serverExchange(rw io.ReadWriter, nonce string, capability wireCapability) (pipeResponse, error) {
	req := pipeRequest{
		Version:    protocolVersion,
		Nonce:      nonce,
		Capability: capability,
	}
	if err := writeMessage(rw, req); err != nil {
		return pipeResponse{}, err
	}
	var resp pipeResponse
	if err := readMessage(rw, &resp); err != nil {
		return pipeResponse{}, err
	}
	if err := validateResponse(resp); err != nil {
		return pipeResponse{}, err
	}
	return resp, nil
}

// helperExchange is the helper side of the one-request protocol. It reads and
// validates exactly one request, dispatches the fixed capability to run, writes
// exactly one response, and returns — it never reads a second request, so a
// replayed or injected follow-up is ignored. A validation failure runs no work
// and returns the error without responding.
func helperExchange(rw io.ReadWriter, nonce string, run func(wireCapability) pipeResponse) error {
	var req pipeRequest
	if err := readMessage(rw, &req); err != nil {
		return err
	}
	if err := validateRequest(req, nonce); err != nil {
		return err
	}
	return writeMessage(rw, run(req.Capability))
}

// responseFromAnalysis projects a path-free analysis result onto the wire
// response. Only stable fields cross the pipe; no raw output or path is carried.
func responseFromAnalysis(res clean.ServicingAnalysisResult) pipeResponse {
	resp := pipeResponse{
		Version:             protocolVersion,
		Outcome:             string(res.Outcome),
		Reason:              res.Reason,
		ReclaimablePackages: res.ReclaimablePackages,
		CleanupRecommended:  res.CleanupRecommended,
	}
	if res.ExitCode != nil {
		resp.HasExitCode = true
		resp.ExitCode = *res.ExitCode
	}
	return resp
}

// responseFromExecute projects a path-free composite execute result onto the
// wire response, adding the mutation restart-required/cancellation-request
// state. No raw output or path is carried.
func responseFromExecute(res clean.ServicingExecuteResult) pipeResponse {
	resp := pipeResponse{
		Version:             protocolVersion,
		Outcome:             string(res.Outcome),
		Reason:              res.Reason,
		ReclaimablePackages: res.ReclaimablePackages,
		CleanupRecommended:  res.CleanupRecommended,
		RestartRequired:     res.RestartRequired,
		CancelRequested:     res.CancelRequested,
	}
	if res.ExitCode != nil {
		resp.HasExitCode = true
		resp.ExitCode = *res.ExitCode
	}
	return resp
}

// resultFromResponse reconstructs a path-free analysis result from a wire
// response. Outcome and reason validity are re-checked by the clean package's
// fail-closed mapping, so an unknown value can never be read as approval.
func resultFromResponse(resp pipeResponse) clean.ServicingAnalysisResult {
	res := clean.ServicingAnalysisResult{
		Outcome:             clean.ServicingOutcome(resp.Outcome),
		Reason:              resp.Reason,
		ReclaimablePackages: resp.ReclaimablePackages,
		CleanupRecommended:  resp.CleanupRecommended,
	}
	if resp.HasExitCode {
		code := resp.ExitCode
		res.ExitCode = &code
	}
	return res
}

// executeResultFromResponse reconstructs a path-free composite execute result
// from a wire response. The clean package's fail-closed mapping re-checks
// outcome validity, so an unknown value can never be read as success.
func executeResultFromResponse(resp pipeResponse) clean.ServicingExecuteResult {
	res := clean.ServicingExecuteResult{
		Outcome:             clean.ServicingOutcome(resp.Outcome),
		Reason:              resp.Reason,
		ReclaimablePackages: resp.ReclaimablePackages,
		CleanupRecommended:  resp.CleanupRecommended,
		RestartRequired:     resp.RestartRequired,
		CancelRequested:     resp.CancelRequested,
	}
	if resp.HasExitCode {
		code := resp.ExitCode
		res.ExitCode = &code
	}
	return res
}

// skipResult builds a path-free skipped analysis result with a stable reason.
func skipResult(reason string) clean.ServicingAnalysisResult {
	return clean.ServicingAnalysisResult{Outcome: clean.ServicingOutcomeSkipped, Reason: reason}
}

// failResult builds a path-free failed analysis result with a stable reason.
func failResult(reason string) clean.ServicingAnalysisResult {
	return clean.ServicingAnalysisResult{Outcome: clean.ServicingOutcomeFailed, Reason: reason}
}

// skipExecuteResult builds a path-free skipped composite execute result.
func skipExecuteResult(reason string) clean.ServicingExecuteResult {
	return clean.ServicingExecuteResult{Outcome: clean.ServicingOutcomeSkipped, Reason: reason}
}

// failExecuteResult builds a path-free failed composite execute result.
func failExecuteResult(reason string) clean.ServicingExecuteResult {
	return clean.ServicingExecuteResult{Outcome: clean.ServicingOutcomeFailed, Reason: reason}
}

// canceledExecuteResult builds a path-free pre-mutation canceled execute result.
func canceledExecuteResult() clean.ServicingExecuteResult {
	return clean.ServicingExecuteResult{Outcome: clean.ServicingOutcomeCanceled, Reason: clean.ServicingReasonContextCanceled}
}
