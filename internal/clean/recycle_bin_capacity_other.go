//go:build !windows

package clean

import "errors"

func recycleBinVolumeCapacity(string) (RecycleBinVolumeConfig, error) {
	return RecycleBinVolumeConfig{}, errors.New("Recycle Bin capacity probing is unsupported on this platform")
}
