package clean

import "testing"

func TestClassifyEvidenceIssueCode(t *testing.T) {
	cases := []struct {
		code string
		want evidenceIssueClass
	}{
		{"context_canceled", evidenceIssueCancel},
		{PreviewReasonContextCanceled, evidenceIssueCancel},
		{PreviewReasonInspectionLimit, evidenceIssueIncomplete},
		{"reparse_point", evidenceIssueIncomplete},
		{runningApplicationDetectionIssueCode, evidenceIssueRunningUnknown},
		{"delete_failed", evidenceIssueOperational},
		{"permission_denied", evidenceIssueOperational},
		{permanentDeleteFailedIssueCode, evidenceIssueOperational},
		{PreviewReasonInspectionFailed, evidenceIssueOperational},
		{PreviewReasonProtectionConfigFailed, evidenceIssueOperational},
		{"protected_path", evidenceIssueSafety},
		{"permanent_deletion_not_authorized", evidenceIssueSafety},
		{"recycle_bin_capacity", evidenceIssueSafety},
		{"", evidenceIssueSafety},
		{"unknown_future_code", evidenceIssueSafety},
	}
	for _, tc := range cases {
		if got := classifyEvidenceIssueCode(tc.code); got != tc.want {
			t.Fatalf("code %q = %v, want %v", tc.code, got, tc.want)
		}
	}
}

func TestMapPreviewOutcomeTaxonomy(t *testing.T) {
	cases := []struct {
		name     string
		factors  categoryEvidenceFactors
		want     CategoryPreviewState
		wantCode string
	}{
		{
			name:    "complete when only safe candidates",
			factors: categoryEvidenceFactors{SuccessCount: 2},
			want:    CategoryPreviewComplete,
		},
		{
			name: "partial when safe plus protected",
			factors: categoryEvidenceFactors{
				SuccessCount:   1,
				ProtectedCount: 1,
			},
			want:     CategoryPreviewPartial,
			wantCode: PreviewReasonProtected,
		},
		{
			name:     "empty when no residual",
			factors:  categoryEvidenceFactors{},
			want:     CategoryPreviewEmpty,
			wantCode: PreviewReasonEmpty,
		},
		{
			name: "skipped all protected",
			factors: categoryEvidenceFactors{
				ProtectedCount: 2,
			},
			want:     CategoryPreviewSkipped,
			wantCode: PreviewReasonProtected,
		},
		{
			name: "skipped running without path-backed skip",
			factors: categoryEvidenceFactors{
				RunningBlocked: true,
			},
			want:     CategoryPreviewSkipped,
			wantCode: PreviewReasonApplicationRunning,
		},
		{
			name: "incomplete cancel",
			factors: categoryEvidenceFactors{
				Canceled: true,
			},
			want:     CategoryPreviewIncomplete,
			wantCode: PreviewReasonContextCanceled,
		},
		{
			name: "incomplete inspection limit",
			factors: categoryEvidenceFactors{
				IncompleteCount:  1,
				DiagnosticReason: PreviewReasonInspectionLimit,
			},
			want:     CategoryPreviewIncomplete,
			wantCode: PreviewReasonInspectionLimit,
		},
		{
			name: "failed operational residual",
			factors: categoryEvidenceFactors{
				OperationalCount: 1,
				DiagnosticReason: PreviewReasonInspectionFailed,
			},
			want:     CategoryPreviewFailed,
			wantCode: PreviewReasonInspectionFailed,
		},
		{
			name: "failed has-any-diagnostic edge without operational count",
			factors: categoryEvidenceFactors{
				HasAnyDiagnostic: true,
				DiagnosticReason: runningApplicationDetectionIssueCode,
			},
			want:     CategoryPreviewFailed,
			wantCode: runningApplicationDetectionIssueCode,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, reason, _ := mapPreviewOutcome(tc.factors)
			if state != tc.want {
				t.Fatalf("state = %q, want %q", state, tc.want)
			}
			if reason != tc.wantCode {
				t.Fatalf("reason = %q, want %q", reason, tc.wantCode)
			}
		})
	}
}

func TestMapExecutionOutcomeTaxonomy(t *testing.T) {
	cases := []struct {
		name    string
		factors categoryEvidenceFactors
		want    CategoryExecutionState
	}{
		{
			name:    "cleaned full success",
			factors: categoryEvidenceFactors{SuccessCount: 3},
			want:    CategoryExecutionCleaned,
		},
		{
			name: "partial success with residual skip",
			factors: categoryEvidenceFactors{
				SuccessCount: 1,
				SkippedCount: 1,
				HasSafetySkip: true,
			},
			want: CategoryExecutionPartial,
		},
		{
			name: "partial success with cancel",
			factors: categoryEvidenceFactors{
				SuccessCount: 1,
				Canceled:     true,
			},
			want: CategoryExecutionPartial,
		},
		{
			name: "partial success with operational fail without skipped count",
			factors: categoryEvidenceFactors{
				SuccessCount:       1,
				HasOperationalFail: true,
			},
			want: CategoryExecutionPartial,
		},
		{
			name:    "empty",
			factors: categoryEvidenceFactors{},
			want:    CategoryExecutionEmpty,
		},
		{
			name: "canceled zero success",
			factors: categoryEvidenceFactors{
				Canceled: true,
			},
			want: CategoryExecutionCanceled,
		},
		{
			name: "failed operational zero success",
			factors: categoryEvidenceFactors{
				HasOperationalFail: true,
			},
			want: CategoryExecutionFailed,
		},
		{
			name: "skipped safety",
			factors: categoryEvidenceFactors{
				SkippedCount:  1,
				HasSafetySkip: true,
			},
			want: CategoryExecutionSkipped,
		},
		{
			name: "cancel beats operational at zero success",
			factors: categoryEvidenceFactors{
				Canceled:           true,
				HasOperationalFail: true,
			},
			want: CategoryExecutionCanceled,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapExecutionOutcome(tc.factors); got != tc.want {
				t.Fatalf("state = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCategoryEvidenceFromResolutionIsPathFree(t *testing.T) {
	factors := categoryEvidenceFromResolution(CategoryResolution{
		Identifier: OpportunityCategoryCrashDumps,
		Candidates: []CandidatePreview{{
			Path:  `C:\Users\secret\CrashDumps\a.dmp`,
			Bytes: 10,
		}},
		SuppressedProtectionPaths: []string{`C:\Users\secret\CrashDumps\protected`},
		Skipped: []SkippedItem{{
			Path:   `C:\Users\secret\CrashDumps\skip`,
			Reason: StructuredIssue{Code: "protected_path", Message: `blocked C:\Users\secret`},
		}},
		Diagnostics: []StructuredIssue{{
			Code:    PreviewReasonInspectionLimit,
			Message: `walk C:\Users\secret\huge: limit`,
			Path:    `C:\Users\secret\huge`,
		}},
	})
	if factors.SuccessCount != 1 || factors.ProtectedCount != 1 || factors.SkippedCount != 1 || factors.IncompleteCount != 1 {
		t.Fatalf("factors = %#v", factors)
	}
	if factors.SkipReasonCode != "protected_path" {
		t.Fatalf("skip reason = %q", factors.SkipReasonCode)
	}
	if factors.DiagnosticReason != PreviewReasonInspectionLimit {
		t.Fatalf("diagnostic reason = %q", factors.DiagnosticReason)
	}
	// Factors must not retain path-bearing fields; only counts and codes.
	state, reason, excluded := mapPreviewOutcome(factors)
	if state != CategoryPreviewPartial || reason != PreviewReasonProtected || excluded != 3 {
		t.Fatalf("mapped = %q/%q/%d", state, reason, excluded)
	}
}
