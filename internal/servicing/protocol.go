package servicing

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// protocolVersion is the single supported coordinator/helper wire version. A
// mismatch on either side fails closed: the helper never guesses intent from an
// unknown protocol, and the coordinator never trusts an unknown response.
const protocolVersion uint32 = 1

// maxMessageBytes bounds a single pipe message. One request and one response are
// small, fixed-shape JSON objects; anything larger is rejected before parsing so
// a malformed or hostile peer cannot exhaust memory.
const maxMessageBytes = 64 * 1024

// nonceBytes is the size of the one-time launch nonce. It binds a request to the
// specific helper launch so a replayed or foreign request fails closed even if
// the pipe name is guessed.
const nonceBytes = 32

// wireCapability is the fixed built-in capability enum carried in a request. It
// is the ONLY instruction the helper accepts: no executable, path, command
// line, or DISM argument is ever transmitted. Exactly two capabilities exist:
// read-only analysis and the composite execute (fresh analysis + guard +
// StartComponentCleanup). There is no standalone start-cleanup capability.
type wireCapability uint8

const (
	wireCapabilityAnalyzeComponentStore        wireCapability = 1
	wireCapabilityExecuteComponentStoreCleanup wireCapability = 2
)

func validWireCapability(c wireCapability) bool {
	return c == wireCapabilityAnalyzeComponentStore || c == wireCapabilityExecuteComponentStoreCleanup
}

// pipeRequest is the one and only request the coordinator sends to the helper.
// It carries the protocol version, the one-time nonce, and a fixed capability
// enum — never a path, command line, or argument.
type pipeRequest struct {
	Version    uint32         `json:"version"`
	Nonce      string         `json:"nonce"`
	Capability wireCapability `json:"capability"`
}

// pipeResponse is the structured servicing result the helper returns. It is
// path-free: only the parsed English analysis fields, a stable Foal-owned
// outcome and reason, an optional DISM exit code, the mutation
// restart-required/cancellation-request state, and an optional post-mutation
// free-space observation. It never carries raw DISM output, OS error text, or a
// package identifier. The observation is the single permitted byte value: an
// approximate external disk reading around a completed exit-0 cleanup, never a
// reclaimable estimate.
type pipeResponse struct {
	Version             uint32 `json:"version"`
	Outcome             string `json:"outcome"`
	Reason              string `json:"reason,omitempty"`
	ReclaimablePackages int    `json:"reclaimable_packages"`
	CleanupRecommended  bool   `json:"cleanup_recommended"`
	HasExitCode         bool   `json:"has_exit_code"`
	ExitCode            int    `json:"exit_code,omitempty"`
	// RestartRequired and CancelRequested are meaningful for the composite
	// execute capability. Analysis leaves them false.
	RestartRequired bool `json:"restart_required,omitempty"`
	CancelRequested bool `json:"cancel_requested,omitempty"`
	// HasObservedFreeBytes distinguishes a measured value (including zero) from
	// "not measured"; ObservedFreeBytes carries the non-negative delta when set.
	HasObservedFreeBytes bool  `json:"has_observed_free_bytes,omitempty"`
	ObservedFreeBytes    int64 `json:"observed_free_bytes,omitempty"`
}

// newNonce returns a fresh random one-time nonce as a hex string.
func newNonce() (string, error) {
	buf := make([]byte, nonceBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// writeMessage encodes v as a single newline-terminated JSON message.
func writeMessage(w io.Writer, v any) error {
	encoded, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(encoded)+1 > maxMessageBytes {
		return errors.New("servicing: message exceeds maximum size")
	}
	encoded = append(encoded, '\n')
	if _, err := w.Write(encoded); err != nil {
		return err
	}
	return nil
}

// readMessage reads exactly one newline-terminated JSON message into v, bounding
// the read so a hostile or malformed peer cannot force an unbounded allocation.
func readMessage(r io.Reader, v any) error {
	reader := bufio.NewReaderSize(io.LimitReader(r, maxMessageBytes), maxMessageBytes)
	line, err := reader.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(line) == 0 {
		return errors.New("servicing: empty message")
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("servicing: decode message: %w", err)
	}
	return nil
}

// validateRequest enforces the one-request protocol contract on the helper side:
// the protocol version must match exactly, the nonce must equal the launch nonce
// (constant-time compared), and the capability must be a known built-in. Any
// mismatch fails closed.
func validateRequest(req pipeRequest, expectedNonce string) error {
	if req.Version != protocolVersion {
		return fmt.Errorf("servicing: unsupported protocol version %d", req.Version)
	}
	if subtle.ConstantTimeCompare([]byte(req.Nonce), []byte(expectedNonce)) != 1 {
		return errors.New("servicing: nonce mismatch")
	}
	if !validWireCapability(req.Capability) {
		return fmt.Errorf("servicing: unknown capability %d", req.Capability)
	}
	return nil
}

// validateResponse enforces the protocol version on the coordinator side.
func validateResponse(resp pipeResponse) error {
	if resp.Version != protocolVersion {
		return fmt.Errorf("servicing: unsupported response protocol version %d", resp.Version)
	}
	return nil
}
