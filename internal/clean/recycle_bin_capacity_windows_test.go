//go:build windows

package clean

import (
	"errors"
	"testing"
)

func TestRecycleBinVolumeCapacityReportsConfiguredCapacityAndCurrentUsage(t *testing.T) {
	const (
		root   = `D:\`
		volume = `\\?\Volume{test-volume}\`
	)
	config, err := recycleBinVolumeCapacityWithDependencies(`D:\cache\candidate`, recycleBinVolumeProbeDependencies{
		volumeIdentity: func(string) (string, string, error) {
			return root, volume, nil
		},
		readConfig: func(gotVolume string) (bool, uint64, error) {
			if gotVolume != volume {
				t.Fatalf("volume = %q, want %q", gotVolume, volume)
			}
			return false, 20, nil
		},
		currentUsage: func(gotRoot string) (int64, error) {
			if gotRoot != root {
				t.Fatalf("root = %q, want %q", gotRoot, root)
			}
			return 75, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.Volume != volume || config.NukeOnDelete || config.MaxCapacity != 20*1024*1024 || config.CurrentUsage != 75 {
		t.Fatalf("config = %#v, want volume/enabled/20 MiB capacity/75 usage", config)
	}
}

func TestRecycleBinVolumeCapacityFailsClosedWhenConfigurationIsUnknown(t *testing.T) {
	const volume = `\\?\Volume{test-volume}\`
	config, err := recycleBinVolumeCapacityWithDependencies(`D:\cache\candidate`, recycleBinVolumeProbeDependencies{
		volumeIdentity: func(string) (string, string, error) {
			return `D:\`, volume, nil
		},
		readConfig: func(string) (bool, uint64, error) {
			return false, 0, errors.New("missing volume configuration")
		},
		currentUsage: func(string) (int64, error) {
			t.Fatal("usage must not be queried after configuration failure")
			return 0, nil
		},
	})
	if err == nil {
		t.Fatal("error = nil, want unknown configuration failure")
	}
	if config.Volume != volume {
		t.Fatalf("volume = %q, want known volume identity %q on failure", config.Volume, volume)
	}
}

func TestRecycleBinVolumeCapacityFailsClosedWhenCurrentUsageIsUnknown(t *testing.T) {
	const volume = `\\?\Volume{test-volume}\`
	config, err := recycleBinVolumeCapacityWithDependencies(`D:\cache\candidate`, recycleBinVolumeProbeDependencies{
		volumeIdentity: func(string) (string, string, error) {
			return `D:\`, volume, nil
		},
		readConfig: func(string) (bool, uint64, error) {
			return false, 200, nil
		},
		currentUsage: func(string) (int64, error) {
			return 0, errors.New("SHQueryRecycleBin failed")
		},
	})
	if err == nil {
		t.Fatal("error = nil, want unknown usage failure")
	}
	if config.Volume != volume || config.MaxCapacity != 200*1024*1024 {
		t.Fatalf("config = %#v, want known volume and configured capacity on usage failure", config)
	}
}
