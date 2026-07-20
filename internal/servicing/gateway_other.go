//go:build !windows

package servicing

import (
	"context"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// NewGateway returns the servicing gateway for the current platform. Off Windows
// there is no component store, DISM, or elevated helper, so analysis fails
// closed with unsupported_platform and never touches the filesystem or opens a
// prompt.
func NewGateway() clean.ServicingGateway { return unsupportedGateway{} }

type unsupportedGateway struct{}

func (unsupportedGateway) AnalyzeComponentStore(context.Context, clean.ServicingAnalysisRequest) clean.ServicingAnalysisResult {
	return skipResult(clean.ServicingReasonUnsupportedPlatform)
}

func (unsupportedGateway) ExecuteComponentStoreCleanup(context.Context, clean.ServicingExecuteRequest) clean.ServicingExecuteResult {
	return skipExecuteResult(clean.ServicingReasonUnsupportedPlatform)
}

// RunHelper is unsupported off Windows; the elevated helper only exists there.
func RunHelper([]string) int { return helperExitUnsupported }
