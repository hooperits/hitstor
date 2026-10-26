//go:build !linux

package fs

// ReadXattrs returns no attributes on platforms where HitStor does not capture
// extended attributes. Windows system state is carried by security descriptors,
// which are handled separately.
func ReadXattrs(path string) (map[string][]byte, error) {
	return nil, nil
}

// WriteXattrs is a no-op on platforms without extended-attribute capture.
func WriteXattrs(path string, attrs map[string][]byte) error {
	return nil
}

// WriteXattrsReport reports every attribute as skipped, since none can be
// applied on this platform.
func WriteXattrsReport(path string, attrs map[string][]byte) ([]string, error) {
	if len(attrs) == 0 {
		return nil, nil
	}
	skipped := make([]string, 0, len(attrs))
	for name := range attrs {
		skipped = append(skipped, name)
	}
	return skipped, nil
}
