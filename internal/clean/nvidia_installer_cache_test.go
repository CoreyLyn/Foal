package clean_test

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

const nvidiaTestTaskID = "c1341b0ededc2dcc770c70a1bfda183e"
const nvidiaTestPayloadName = "566.03-desktop-win10-win11-64bit-international-dch-whql.exe"
const nvidiaTestDownloadURL = "https://download.nvidia.com/Windows/566.03/566.03-desktop-win10-win11-64bit-international-dch-whql.exe"

// nvidiaStatusRec mirrors the safety-relevant status.json record fields the
// resolver reads. Pointer status/type let tests omit those fields.
type nvidiaStatusRec struct {
	TaskID        string `json:"taskId"`
	Status        *int   `json:"status,omitempty"`
	DownloadType  *int   `json:"downloadType,omitempty"`
	Version       string `json:"version"`
	Checksum      string `json:"checksum"`
	FileLocation  string `json:"fileLocation"`
	DownloadURL   string `json:"downloadUrl"`
	ExtractedPath string `json:"extractedPath,omitempty"`
}

func intPtr(v int) *int { return &v }

func nvidiaIdleActivity(context.Context) clean.NVIDIAActivityState {
	return clean.NVIDIAActivityState{Status: clean.NVIDIAActivityIdle}
}

func nvidiaOKSignature(string) error { return nil }

func nvidiaOKForensics(string) (clean.NVIDIAPayloadForensics, error) {
	return clean.NVIDIAPayloadForensics{HardLinkCount: 1, HasAlternateDataStreams: false}, nil
}

// nvidiaFixture is a hermetic Downloader root with one valid completed
// display-driver task. Tests mutate it before resolving.
type nvidiaFixture struct {
	root     string
	taskDir  string
	payload  string
	checksum string
	now      time.Time
}

func writeNVIDIAStatusRecords(t *testing.T, root string, records []nvidiaStatusRec) {
	t.Helper()
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "status.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

// buildValidNVIDIAFixture creates a fully valid task tree: root/status.json with
// one status=2 display-driver record, a task directory holding exactly the one
// payload file with a matching checksum and an old (non-recent) write time.
func buildValidNVIDIAFixture(t *testing.T) nvidiaFixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "Downloader")
	taskDir := filepath.Join(root, nvidiaTestTaskID)
	if err := os.MkdirAll(taskDir, 0700); err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(taskDir, nvidiaTestPayloadName)
	body := []byte("nvidia-display-driver-payload-bytes")
	if err := os.WriteFile(payload, body, 0600); err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum(body)
	checksum := hex.EncodeToString(sum[:])

	now := time.Now().Truncate(time.Second)
	// Payload written two hours ago: outside the one-hour recent-write window.
	old := now.Add(-2 * time.Hour)
	if err := os.Chtimes(payload, old, old); err != nil {
		t.Fatal(err)
	}

	writeNVIDIAStatusRecords(t, root, []nvidiaStatusRec{{
		TaskID:       nvidiaTestTaskID,
		Status:       intPtr(2),
		DownloadType: intPtr(1),
		Version:      "566.03",
		Checksum:     checksum,
		FileLocation: payload,
		DownloadURL:  nvidiaTestDownloadURL,
	}})

	return nvidiaFixture{root: root, taskDir: taskDir, payload: payload, checksum: checksum, now: now}
}

func nvidiaResolveOptions(fx nvidiaFixture) clean.Options {
	return clean.Options{
		NVIDIAInstallerCacheDiscoveryOptions: clean.NVIDIAInstallerCacheDiscoveryOptions{
			Root:                    fx.root,
			Now:                     fx.now,
			DetectActivity:          nvidiaIdleActivity,
			VerifyPayloadSignature:  nvidiaOKSignature,
			InspectPayloadForensics: nvidiaOKForensics,
		},
	}
}

func resolveNVIDIA(t *testing.T, opts clean.Options) clean.CategoryResolution {
	t.Helper()
	resolution, err := clean.ResolveCategory(context.Background(), opts, clean.CategoryNVIDIAInstallerCache)
	if err != nil {
		t.Fatalf("ResolveCategory error: %v", err)
	}
	return resolution
}

func TestNVIDIAInstallerCache_ValidCandidate(t *testing.T) {
	fx := buildValidNVIDIAFixture(t)
	resolution := resolveNVIDIA(t, nvidiaResolveOptions(fx))

	if len(resolution.OptInCandidates) != 1 {
		t.Fatalf("opt-in candidates = %d, want 1 (%#v)", len(resolution.OptInCandidates), resolution.OptInCandidates)
	}
	candidate := resolution.OptInCandidates[0]
	if !strings.EqualFold(candidate.Path, fx.taskDir) {
		t.Fatalf("candidate path = %q, want task dir %q", candidate.Path, fx.taskDir)
	}
	if candidate.Category != clean.CategoryNVIDIAInstallerCache {
		t.Fatalf("candidate category = %q", candidate.Category)
	}
	if candidate.PlannedAction != string(clean.PlannedActionMoveToRecycleBin) {
		t.Fatalf("planned action = %q, want move_to_recycle_bin", candidate.PlannedAction)
	}
	if candidate.Bytes <= 0 {
		t.Fatalf("candidate bytes = %d, want > 0", candidate.Bytes)
	}
	if len(resolution.Skipped) != 0 {
		t.Fatalf("unexpected skips: %#v", resolution.Skipped)
	}
	// Idle activity is projected for reporting.
	foundIdle := false
	for _, state := range resolution.RunningStates {
		if state.Application == clean.ApplicationNVIDIA && state.State == clean.RunningApplicationStateIdle {
			foundIdle = true
		}
	}
	if !foundIdle {
		t.Fatalf("expected idle NVIDIA running state, got %#v", resolution.RunningStates)
	}
}

// TestNVIDIAInstallerCache_RejectedStates asserts that each single deviation from
// the valid fixture produces no candidate (silent fail-closed).
func TestNVIDIAInstallerCache_RejectedStates(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, fx nvidiaFixture, opts *clean.Options)
	}{
		{"status not completed", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			writeNVIDIAStatusRecords(t, fx.root, []nvidiaStatusRec{validRecordWith(fx, func(r *nvidiaStatusRec) { r.Status = intPtr(3) })})
		}},
		{"missing status field", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			writeNVIDIAStatusRecords(t, fx.root, []nvidiaStatusRec{validRecordWith(fx, func(r *nvidiaStatusRec) { r.Status = nil })})
		}},
		{"non-driver download type", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			writeNVIDIAStatusRecords(t, fx.root, []nvidiaStatusRec{validRecordWith(fx, func(r *nvidiaStatusRec) { r.DownloadType = intPtr(2) })})
		}},
		{"empty version", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			writeNVIDIAStatusRecords(t, fx.root, []nvidiaStatusRec{validRecordWith(fx, func(r *nvidiaStatusRec) { r.Version = "" })})
		}},
		{"empty checksum", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			writeNVIDIAStatusRecords(t, fx.root, []nvidiaStatusRec{validRecordWith(fx, func(r *nvidiaStatusRec) { r.Checksum = "" })})
		}},
		{"wrong checksum", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			writeNVIDIAStatusRecords(t, fx.root, []nvidiaStatusRec{validRecordWith(fx, func(r *nvidiaStatusRec) {
				r.Checksum = strings.Repeat("a", 32)
			})})
		}},
		{"non-nvidia url host", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			writeNVIDIAStatusRecords(t, fx.root, []nvidiaStatusRec{validRecordWith(fx, func(r *nvidiaStatusRec) {
				r.DownloadURL = "https://download.evil.com/driver.exe"
			})})
		}},
		{"non-https url", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			writeNVIDIAStatusRecords(t, fx.root, []nvidiaStatusRec{validRecordWith(fx, func(r *nvidiaStatusRec) {
				r.DownloadURL = "http://download.nvidia.com/driver.exe"
			})})
		}},
		{"url with userinfo", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			writeNVIDIAStatusRecords(t, fx.root, []nvidiaStatusRec{validRecordWith(fx, func(r *nvidiaStatusRec) {
				r.DownloadURL = "https://user:pass@download.nvidia.com/driver.exe"
			})})
		}},
		{"non-empty extracted path", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			writeNVIDIAStatusRecords(t, fx.root, []nvidiaStatusRec{validRecordWith(fx, func(r *nvidiaStatusRec) {
				r.ExtractedPath = filepath.Join(fx.taskDir, "extracted")
			})})
		}},
		{"file location traversal", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			writeNVIDIAStatusRecords(t, fx.root, []nvidiaStatusRec{validRecordWith(fx, func(r *nvidiaStatusRec) {
				r.FileLocation = filepath.Join(fx.taskDir, "..", "escape.exe")
			})})
		}},
		{"file location alternate stream", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			writeNVIDIAStatusRecords(t, fx.root, []nvidiaStatusRec{validRecordWith(fx, func(r *nvidiaStatusRec) {
				r.FileLocation = fx.payload + ":hidden"
			})})
		}},
		{"duplicate task ids", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			rec := validRecordWith(fx, nil)
			writeNVIDIAStatusRecords(t, fx.root, []nvidiaStatusRec{rec, rec})
		}},
		{"no matching record", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			writeNVIDIAStatusRecords(t, fx.root, []nvidiaStatusRec{validRecordWith(fx, func(r *nvidiaStatusRec) {
				r.TaskID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			})})
		}},
		{"extra file in task dir", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			if err := os.WriteFile(filepath.Join(fx.taskDir, "extra.dat"), []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"subdirectory in task dir", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			if err := os.MkdirAll(filepath.Join(fx.taskDir, "sub"), 0700); err != nil {
				t.Fatal(err)
			}
		}},
		{"recent write", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			recent := fx.now.Add(-10 * time.Minute)
			if err := os.Chtimes(fx.payload, recent, recent); err != nil {
				t.Fatal(err)
			}
		}},
		{"future write", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			future := fx.now.Add(2 * time.Hour)
			if err := os.Chtimes(fx.payload, future, future); err != nil {
				t.Fatal(err)
			}
		}},
		{"multiple hard links", func(t *testing.T, _ nvidiaFixture, opts *clean.Options) {
			opts.NVIDIAInstallerCacheDiscoveryOptions.InspectPayloadForensics = func(string) (clean.NVIDIAPayloadForensics, error) {
				return clean.NVIDIAPayloadForensics{HardLinkCount: 2}, nil
			}
		}},
		{"alternate data streams", func(t *testing.T, _ nvidiaFixture, opts *clean.Options) {
			opts.NVIDIAInstallerCacheDiscoveryOptions.InspectPayloadForensics = func(string) (clean.NVIDIAPayloadForensics, error) {
				return clean.NVIDIAPayloadForensics{HardLinkCount: 1, HasAlternateDataStreams: true}, nil
			}
		}},
		{"forensics error", func(t *testing.T, _ nvidiaFixture, opts *clean.Options) {
			opts.NVIDIAInstallerCacheDiscoveryOptions.InspectPayloadForensics = func(string) (clean.NVIDIAPayloadForensics, error) {
				return clean.NVIDIAPayloadForensics{}, os.ErrPermission
			}
		}},
		{"invalid signature", func(t *testing.T, _ nvidiaFixture, opts *clean.Options) {
			opts.NVIDIAInstallerCacheDiscoveryOptions.VerifyPayloadSignature = func(string) error {
				return os.ErrInvalid
			}
		}},
		{"gfeupdate references task", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			gfe := `{"latest":"latest\\setup.exe","pending":"` + nvidiaTestTaskID + `"}`
			if err := os.WriteFile(filepath.Join(fx.root, "gfeupdate.json"), []byte(gfe), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"non-hex task directory", func(t *testing.T, fx nvidiaFixture, _ *clean.Options) {
			// Rename the task directory to a non-hex name; the record no longer
			// matches an accepted 32-hex directory.
			other := filepath.Join(fx.root, "not-a-hex-directory-name-here!!")
			if err := os.Rename(fx.taskDir, other); err != nil {
				t.Fatal(err)
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := buildValidNVIDIAFixture(t)
			opts := nvidiaResolveOptions(fx)
			tc.mutate(t, fx, &opts)
			resolution := resolveNVIDIA(t, opts)
			if len(resolution.OptInCandidates) != 0 {
				t.Fatalf("expected no candidate, got %#v", resolution.OptInCandidates)
			}
		})
	}
}

func validRecordWith(fx nvidiaFixture, mutate func(*nvidiaStatusRec)) nvidiaStatusRec {
	rec := nvidiaStatusRec{
		TaskID:       nvidiaTestTaskID,
		Status:       intPtr(2),
		DownloadType: intPtr(1),
		Version:      "566.03",
		Checksum:     fx.checksum,
		FileLocation: fx.payload,
		DownloadURL:  nvidiaTestDownloadURL,
	}
	if mutate != nil {
		mutate(&rec)
	}
	return rec
}

func TestNVIDIAInstallerCache_ActivityGateSkipsCategory(t *testing.T) {
	for _, status := range []clean.NVIDIAActivityStatus{clean.NVIDIAActivityRunning, clean.NVIDIAActivityUnknown} {
		t.Run(string(status), func(t *testing.T) {
			fx := buildValidNVIDIAFixture(t)
			opts := nvidiaResolveOptions(fx)
			opts.NVIDIAInstallerCacheDiscoveryOptions.DetectActivity = func(context.Context) clean.NVIDIAActivityState {
				return clean.NVIDIAActivityState{Status: status}
			}
			resolution := resolveNVIDIA(t, opts)
			if len(resolution.OptInCandidates) != 0 {
				t.Fatalf("expected no candidate on %s activity, got %#v", status, resolution.OptInCandidates)
			}
			if len(resolution.Skipped) != 1 {
				t.Fatalf("expected 1 whole-category skip, got %#v", resolution.Skipped)
			}
			if resolution.Skipped[0].Reason.Code != "nvidia_application_running" {
				t.Fatalf("skip reason = %q, want nvidia_application_running", resolution.Skipped[0].Reason.Code)
			}
		})
	}
}

func TestNVIDIAInstallerCache_MissingRootIsEmpty(t *testing.T) {
	fx := buildValidNVIDIAFixture(t)
	opts := nvidiaResolveOptions(fx)
	opts.NVIDIAInstallerCacheDiscoveryOptions.Root = filepath.Join(t.TempDir(), "does-not-exist")
	resolution := resolveNVIDIA(t, opts)
	if len(resolution.OptInCandidates) != 0 || len(resolution.Skipped) != 0 || len(resolution.Diagnostics) != 0 {
		t.Fatalf("missing root should be silent empty: %#v", resolution)
	}
}

func TestNVIDIAInstallerCache_ProtectedRootSuppressed(t *testing.T) {
	fx := buildValidNVIDIAFixture(t)
	opts := nvidiaResolveOptions(fx)
	opts.Validator = pathsafe.NewValidator([]string{fx.root})
	resolution := resolveNVIDIA(t, opts)
	if len(resolution.OptInCandidates) != 0 {
		t.Fatalf("protected root must yield no candidate, got %#v", resolution.OptInCandidates)
	}
	if len(resolution.SuppressedProtectionPaths) == 0 {
		t.Fatal("expected suppressed protection path for protected root")
	}
}

func TestNVIDIAInstallerCache_ProtectedCandidateSuppressedBeforeTotals(t *testing.T) {
	fx := buildValidNVIDIAFixture(t)
	opts := nvidiaResolveOptions(fx)
	opts.Validator = pathsafe.NewValidator([]string{fx.taskDir})
	resolution := resolveNVIDIA(t, opts)
	if len(resolution.OptInCandidates) != 0 {
		t.Fatalf("protected candidate must be suppressed, got %#v", resolution.OptInCandidates)
	}
	if len(resolution.SuppressedProtectionPaths) == 0 {
		t.Fatal("expected suppressed protection path for protected candidate")
	}
}

func TestNVIDIAInstallerCache_MalformedStatusSkips(t *testing.T) {
	fx := buildValidNVIDIAFixture(t)
	if err := os.WriteFile(filepath.Join(fx.root, "status.json"), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	resolution := resolveNVIDIA(t, nvidiaResolveOptions(fx))
	if len(resolution.OptInCandidates) != 0 {
		t.Fatalf("malformed registry must yield no candidate, got %#v", resolution.OptInCandidates)
	}
	if len(resolution.Diagnostics) == 0 {
		t.Fatal("expected a diagnostic for malformed status registry")
	}
}

func TestNVIDIAInstallerCache_MissingStatusIsEmpty(t *testing.T) {
	fx := buildValidNVIDIAFixture(t)
	if err := os.Remove(filepath.Join(fx.root, "status.json")); err != nil {
		t.Fatal(err)
	}
	resolution := resolveNVIDIA(t, nvidiaResolveOptions(fx))
	if len(resolution.OptInCandidates) != 0 || len(resolution.Diagnostics) != 0 {
		t.Fatalf("missing status.json should be silent empty: %#v", resolution)
	}
}
