//go:build windows

package servicing

import (
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/CoreyLyn/Foal/internal/clean"
)

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
		helperDone <- helperExchange(clientConn, nonce, func() clean.ServicingAnalysisResult {
			analyzed <- struct{}{}
			return readyAnalyze()
		})
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

	resp, err := serverExchange(serverConn, nonce)
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
		helperDone <- helperExchange(clientConn, "helper-launch-nonce", func() clean.ServicingAnalysisResult {
			analyzed = true
			return readyAnalyze()
		})
	}()

	if err := connectServerPipe(server, 10*time.Second); err != nil {
		t.Fatalf("connectServerPipe: %v", err)
	}
	// Coordinator sends a different nonce.
	_, _ = serverExchange(serverConn, "coordinator-nonce")

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
