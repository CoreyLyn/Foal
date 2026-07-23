package analyze

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Browse child kinds (shared by BrowseChild and ChildObservation).
const (
	BrowseKindFile      = "file"
	BrowseKindDirectory = "directory"
	BrowseKindReparse   = "reparse_point"
)

// Browse measurement states for on-demand disk browse. The shared Analyze core
// owns transitions; the TUI only projects them.
const (
	BrowseStateScanning   = "scanning"
	BrowseStateComplete   = "complete"
	BrowseStatePartial    = "partial"
	BrowseStateIncomplete = "incomplete"
	BrowseStateSkipped    = "skipped"
)

// Stable skip / omission reasons used in SkipReason and SkipAggregates.
const (
	SkipReasonReparsePoint     = "reparse_point"
	SkipReasonPermissionDenied = "permission_denied"
	SkipReasonNotFound         = "not_found"
	SkipReasonReadError        = "read_error"
	SkipReasonCanceled         = "canceled"
	SkipReasonHardLimit        = "hard_limit"
)

// DefaultObservationMinInterval is the default UI-safe cadence for non-terminal
// Scanning observations. Cadence is never part of correctness; terminals always emit.
const DefaultObservationMinInterval = 100 * time.Millisecond

// SkipAggregate is a path-free count of one stable omission reason under a child.
type SkipAggregate struct {
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}

// ChildObservation is one path-scoped incremental or terminal measurement update.
// It never carries an unbounded descendant-path error list.
type ChildObservation struct {
	Name           string          `json:"name"`
	Path           string          `json:"path"`
	Kind           string          `json:"kind"`
	Bytes          int64           `json:"bytes"`
	FileCount      int64           `json:"file_count"`
	DirectoryCount int64           `json:"directory_count"`
	Classification string          `json:"classification,omitempty"`
	State          string          `json:"state"`
	SkipReason     string          `json:"skip_reason,omitempty"`
	SkipAggregates []SkipAggregate `json:"skip_aggregates,omitempty"`
	Hidden         bool            `json:"hidden,omitempty"`
	System         bool            `json:"system,omitempty"`
	Navigable      bool            `json:"navigable"`
	// Terminal is true when State will not change further for this path in the
	// current measurement attempt (Complete, Partial, Incomplete, or Skipped).
	Terminal bool `json:"terminal"`
}

// IsTerminalBrowseState reports whether state is a terminal measurement state.
func IsTerminalBrowseState(state string) bool {
	switch state {
	case BrowseStateComplete, BrowseStatePartial, BrowseStateIncomplete, BrowseStateSkipped:
		return true
	default:
		return false
	}
}

// SizeIsLowerBound reports whether observed bytes are a guaranteed lower bound only.
// Partial and Incomplete sizes must render as ">= observed"; percentages never use ">=".
func SizeIsLowerBound(state string) bool {
	return state == BrowseStatePartial || state == BrowseStateIncomplete
}

// PercentIsApproximate reports whether a share of the current location total must
// be labeled approximate/observed. Exact wording is allowed only when the location
// total is complete and the child itself is complete.
func PercentIsApproximate(childState string, locationComplete bool) bool {
	if !locationComplete {
		return true
	}
	return childState != BrowseStateComplete
}

// LocationMeasurementComplete reports whether every contributing child finished
// with a complete, non-omitted measurement (no Scanning/Partial/Incomplete/Skipped).
func LocationMeasurementComplete(children []BrowseChild) bool {
	if len(children) == 0 {
		return true
	}
	for _, c := range children {
		if c.State != BrowseStateComplete {
			return false
		}
	}
	return true
}

// ObservedLocationBytes sums observed logical bytes across direct children.
func ObservedLocationBytes(children []BrowseChild) int64 {
	var total int64
	for _, c := range children {
		total += c.Bytes
	}
	return total
}

// FormatSizeToken returns a display token for observed bytes using a supplied
// byte formatter. Partial/Incomplete get a ">=" prefix; Scanning and others do not.
func FormatSizeToken(bytes int64, state string, formatBytes func(int64) string) string {
	if formatBytes == nil {
		formatBytes = func(n int64) string { return fmt.Sprintf("%d B", n) }
	}
	token := formatBytes(bytes)
	if SizeIsLowerBound(state) {
		return ">= " + token
	}
	return token
}

// sharePercentColWidth is the fixed rune width of FormatSharePercent tokens so
// the '%' sign lines up vertically across ranked rows (right-aligned).
// Widest token is "100.0%" (6).
const sharePercentColWidth = 6

// shareTenths returns childBytes as tenths of a percent of observedTotal
// (units of 0.1%). 250 means 25.0%. Zero total or non-positive child → 0.
func shareTenths(childBytes, observedTotal int64) int64 {
	if observedTotal <= 0 || childBytes <= 0 {
		return 0
	}
	tenths := (childBytes * 1000) / observedTotal
	if tenths < 0 {
		return 0
	}
	if tenths > 1000 {
		return 1000
	}
	return tenths
}

// FormatSharePercent formats childBytes as a one-decimal percentage of
// observedTotal (e.g. "25.0%"). Shares never use "~" or a ">=" percentage
// marker. Sub-0.1% contributions render as "<0.1%". Tokens are right-aligned
// to sharePercentColWidth so the '%' column is vertically aligned in lists.
// Returns "" when total is zero. childState and locationComplete remain for
// API compatibility; they no longer change wording.
func FormatSharePercent(childBytes, observedTotal int64, childState string, locationComplete bool) string {
	if observedTotal <= 0 {
		return ""
	}
	tenths := shareTenths(childBytes, observedTotal)
	var raw string
	if tenths < 1 {
		raw = "<0.1%"
	} else {
		raw = fmt.Sprintf("%d.%d%%", tenths/10, tenths%10)
	}
	if n := len(raw); n < sharePercentColWidth {
		return strings.Repeat(" ", sharePercentColWidth-n) + raw
	}
	return raw
}

// FormatFocusedDetail builds a compact, path-free detail line for the focused child.
// It exposes state, logical bytes, file/directory counts, skipped total, and
// aggregated stable skip reasons without listing descendant paths.
// formatBytes is optional; nil uses a plain "N B" formatter.
func FormatFocusedDetail(child BrowseChild) string {
	return FormatFocusedDetailWith(child, nil)
}

// FormatFocusedDetailWith is FormatFocusedDetail with an injectable byte formatter
// so the TUI can reuse cleanFormatBytes without inventing a second size scale.
func FormatFocusedDetailWith(child BrowseChild, formatBytes func(int64) string) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("state=%s", child.State))
	// Logical observed bytes (lower-bound token for Partial/Incomplete).
	parts = append(parts, "bytes="+FormatSizeToken(child.Bytes, child.State, formatBytes))
	parts = append(parts, fmt.Sprintf("files=%d", child.FileCount))
	parts = append(parts, fmt.Sprintf("dirs=%d", child.DirectoryCount))
	if child.State == BrowseStateSkipped && child.SkipReason != "" {
		parts = append(parts, "reason="+child.SkipReason)
	}
	if len(child.SkipAggregates) > 0 {
		// Stable reason order for deterministic UI/tests.
		aggs := append([]SkipAggregate(nil), child.SkipAggregates...)
		sort.Slice(aggs, func(i, j int) bool {
			if aggs[i].Reason == aggs[j].Reason {
				return aggs[i].Count < aggs[j].Count
			}
			return aggs[i].Reason < aggs[j].Reason
		})
		var skipParts []string
		var skippedTotal int64
		for _, a := range aggs {
			if a.Count <= 0 || a.Reason == "" {
				continue
			}
			skippedTotal += a.Count
			skipParts = append(skipParts, fmt.Sprintf("%s×%d", a.Reason, a.Count))
		}
		if skippedTotal > 0 {
			parts = append(parts, fmt.Sprintf("skipped_total=%d", skippedTotal))
		}
		if len(skipParts) > 0 {
			parts = append(parts, "skips="+strings.Join(skipParts, ","))
		}
	}
	return strings.Join(parts, " · ")
}

// observationThrottle limits non-terminal emissions to a UI-safe cadence.
// Terminal observations always pass. Timing is never part of correctness.
type observationThrottle struct {
	minInterval time.Duration
	lastEmit    time.Time
	// now is injectable for tests; nil uses time.Now.
	now func() time.Time
}

func newObservationThrottle(minInterval time.Duration) *observationThrottle {
	if minInterval < 0 {
		minInterval = 0
	}
	return &observationThrottle{minInterval: minInterval}
}

func (t *observationThrottle) allow(terminal bool) bool {
	if t == nil {
		return true
	}
	if terminal {
		return true
	}
	now := time.Now()
	if t.now != nil {
		now = t.now()
	}
	if t.lastEmit.IsZero() || now.Sub(t.lastEmit) >= t.minInterval {
		t.lastEmit = now
		return true
	}
	return false
}
