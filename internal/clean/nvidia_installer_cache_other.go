//go:build !windows

package clean

import (
	"context"
	"errors"
)

// errNVIDIAUnsupportedPlatform is the fail-closed error for NVIDIA installer
// cache signature and forensic checks on non-Windows platforms. Discovery keeps
// compiling and every gate fails closed so no candidate is ever produced.
var errNVIDIAUnsupportedPlatform = errors.New("nvidia installer cache validation is unsupported on this platform")

// productionDetectNVIDIAActivity fails closed to Unknown off Windows so the whole
// category is skipped rather than guessed idle.
func productionDetectNVIDIAActivity(context.Context) NVIDIAActivityState {
	return NVIDIAActivityState{Status: NVIDIAActivityUnknown, Message: "NVIDIA activity detection is unsupported on this platform"}
}

// productionVerifyNVIDIASignature fails closed off Windows: Authenticode
// verification is only available through Windows trust services.
func productionVerifyNVIDIASignature(string) error {
	return errNVIDIAUnsupportedPlatform
}

// productionInspectNVIDIAPayloadForensics fails closed off Windows so hard-link
// and alternate-data-stream identity can never be assumed safe.
func productionInspectNVIDIAPayloadForensics(string) (NVIDIAPayloadForensics, error) {
	return NVIDIAPayloadForensics{}, errNVIDIAUnsupportedPlatform
}
