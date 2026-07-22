//go:build !windows

package analyze

// platformDriveProbe is a fail-closed non-Windows stub: no local drive letters.
type platformDriveProbe struct{}

func (platformDriveProbe) LogicalDriveMask() (uint32, error) {
	return 0, nil
}

func (platformDriveProbe) SupportedKind(root string) (VolumeKind, bool) {
	_ = root
	return "", false
}

func (platformDriveProbe) ProbeMetadata(root string) VolumeMetadata {
	_ = root
	return VolumeMetadata{}
}
