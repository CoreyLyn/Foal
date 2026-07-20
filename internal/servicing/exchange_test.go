package servicing

import (
	"net"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// runExchange wires a coordinator (serverExchange) and helper (helperExchange)
// over an in-memory duplex pipe so the one-request protocol and authentication
// can be exercised without a real named pipe, UAC, or DISM.
func runExchange(t *testing.T, serverNonce, helperNonce string, analyze func() clean.ServicingAnalysisResult) (pipeResponse, error, error) {
	t.Helper()
	serverConn, helperConn := net.Pipe()
	defer serverConn.Close()
	defer helperConn.Close()

	helperErr := make(chan error, 1)
	go func() {
		helperErr <- helperExchange(helperConn, helperNonce, analyzeDispatch(analyze))
	}()

	_ = serverConn.SetDeadline(time.Now().Add(5 * time.Second))
	resp, serverError := serverExchange(serverConn, serverNonce, wireCapabilityAnalyzeComponentStore)

	var hErr error
	select {
	case hErr = <-helperErr:
	case <-time.After(5 * time.Second):
		t.Fatal("helper exchange did not return")
	}
	return resp, serverError, hErr
}

// analyzeDispatch adapts a read-only analyze function to the capability
// dispatcher the helper expects. The capability is ignored because these tests
// only exercise the analysis path.
func analyzeDispatch(analyze func() clean.ServicingAnalysisResult) func(wireCapability) pipeResponse {
	return func(wireCapability) pipeResponse {
		return responseFromAnalysis(analyze())
	}
}

func readyAnalyze() clean.ServicingAnalysisResult {
	exit0 := 0
	return clean.ServicingAnalysisResult{
		Outcome:             clean.ServicingOutcomeReady,
		ReclaimablePackages: 4,
		CleanupRecommended:  true,
		ExitCode:            &exit0,
	}
}

func TestExchangeHappyPath(t *testing.T) {
	nonce := "sharednonce"
	resp, serverErr, helperErr := runExchange(t, nonce, nonce, readyAnalyze)
	if serverErr != nil || helperErr != nil {
		t.Fatalf("exchange errors: server=%v helper=%v", serverErr, helperErr)
	}
	res := resultFromResponse(resp)
	if res.Outcome != clean.ServicingOutcomeReady || res.ReclaimablePackages != 4 || !res.CleanupRecommended {
		t.Fatalf("response = %#v", res)
	}
	if res.ExitCode == nil || *res.ExitCode != 0 {
		t.Fatalf("response exit = %#v", res.ExitCode)
	}
}

func TestExchangeNonceMismatchFailsClosed(t *testing.T) {
	analyzed := false
	analyze := func() clean.ServicingAnalysisResult {
		analyzed = true
		return readyAnalyze()
	}
	_, _, helperErr := runExchange(t, "coordinator-nonce", "helper-launch-nonce", analyze)
	if helperErr == nil {
		t.Fatal("helper accepted a mismatched nonce")
	}
	if analyzed {
		t.Fatal("helper ran analysis despite nonce mismatch")
	}
}

func TestExchangeVersionMismatchFailsClosed(t *testing.T) {
	serverConn, helperConn := net.Pipe()
	defer serverConn.Close()
	defer helperConn.Close()

	analyzed := false
	helperErr := make(chan error, 1)
	go func() {
		helperErr <- helperExchange(helperConn, "n", analyzeDispatch(func() clean.ServicingAnalysisResult {
			analyzed = true
			return readyAnalyze()
		}))
	}()

	// Send a well-formed request with an unsupported protocol version.
	_ = serverConn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := writeMessage(serverConn, pipeRequest{Version: protocolVersion + 1, Nonce: "n", Capability: wireCapabilityAnalyzeComponentStore}); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case err := <-helperErr:
		if err == nil {
			t.Fatal("helper accepted an unsupported protocol version")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("helper did not return")
	}
	if analyzed {
		t.Fatal("helper ran analysis despite version mismatch")
	}
}

func TestExchangeUnknownCapabilityFailsClosed(t *testing.T) {
	serverConn, helperConn := net.Pipe()
	defer serverConn.Close()
	defer helperConn.Close()

	helperErr := make(chan error, 1)
	go func() {
		helperErr <- helperExchange(helperConn, "n", analyzeDispatch(readyAnalyze))
	}()
	_ = serverConn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := writeMessage(serverConn, pipeRequest{Version: protocolVersion, Nonce: "n", Capability: wireCapability(99)}); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case err := <-helperErr:
		if err == nil {
			t.Fatal("helper accepted an unknown capability")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("helper did not return")
	}
}

// TestExchangeRejectsSecondRequest proves the helper processes exactly one
// request: after responding it returns, so a replayed second request is never
// read or acted upon.
func TestExchangeRejectsSecondRequest(t *testing.T) {
	serverConn, helperConn := net.Pipe()
	defer serverConn.Close()
	defer helperConn.Close()

	calls := 0
	done := make(chan error, 1)
	go func() {
		done <- helperExchange(helperConn, "n", analyzeDispatch(func() clean.ServicingAnalysisResult {
			calls++
			return readyAnalyze()
		}))
	}()

	_ = serverConn.SetDeadline(time.Now().Add(5 * time.Second))
	var resp pipeResponse
	if _, err, hErr := func() (pipeResponse, error, error) {
		if err := writeMessage(serverConn, pipeRequest{Version: protocolVersion, Nonce: "n", Capability: wireCapabilityAnalyzeComponentStore}); err != nil {
			return resp, err, nil
		}
		if err := readMessage(serverConn, &resp); err != nil {
			return resp, err, nil
		}
		return resp, nil, <-done
	}(); err != nil || hErr != nil {
		t.Fatalf("first exchange: err=%v helperErr=%v", err, hErr)
	}
	if calls != 1 {
		t.Fatalf("analyze called %d times, want 1", calls)
	}

	// A second request must not be serviced: the helper has already returned, so
	// the write either errors or is never read/acted upon.
	_ = serverConn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	_ = writeMessage(serverConn, pipeRequest{Version: protocolVersion, Nonce: "n", Capability: wireCapabilityAnalyzeComponentStore})
	if calls != 1 {
		t.Fatalf("analyze called %d times after second request, want 1", calls)
	}
}

func TestExchangeMalformedMessageFailsClosed(t *testing.T) {
	serverConn, helperConn := net.Pipe()
	defer serverConn.Close()
	defer helperConn.Close()

	helperErr := make(chan error, 1)
	go func() {
		helperErr <- helperExchange(helperConn, "n", analyzeDispatch(readyAnalyze))
	}()
	_ = serverConn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := serverConn.Write([]byte("{ not json \n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case err := <-helperErr:
		if err == nil {
			t.Fatal("helper accepted malformed message")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("helper did not return")
	}
}

func TestReadMessageRejectsUnknownFields(t *testing.T) {
	serverConn, helperConn := net.Pipe()
	defer serverConn.Close()
	defer helperConn.Close()
	go func() {
		_, _ = serverConn.Write([]byte(`{"version":1,"nonce":"n","capability":1,"extra":"x"}` + "\n"))
	}()
	var req pipeRequest
	err := readMessage(helperConn, &req)
	if err == nil {
		t.Fatal("readMessage accepted an unknown field")
	}
}

func TestNewNonceIsRandomAndSized(t *testing.T) {
	a, err := newNonce()
	if err != nil {
		t.Fatal(err)
	}
	b, err := newNonce()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("nonces are not unique")
	}
	if len(a) != nonceBytes*2 { // hex encoding doubles the length
		t.Fatalf("nonce length = %d, want %d", len(a), nonceBytes*2)
	}
}

func TestValidateRequestErrors(t *testing.T) {
	if err := validateRequest(pipeRequest{Version: protocolVersion, Nonce: "n", Capability: wireCapabilityAnalyzeComponentStore}, "n"); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	if err := validateRequest(pipeRequest{Version: protocolVersion, Nonce: "wrong", Capability: wireCapabilityAnalyzeComponentStore}, "n"); err == nil {
		t.Fatal("nonce mismatch accepted")
	}
	if err := validateRequest(pipeRequest{Version: 99, Nonce: "n", Capability: wireCapabilityAnalyzeComponentStore}, "n"); err == nil {
		t.Fatal("version mismatch accepted")
	}
}
