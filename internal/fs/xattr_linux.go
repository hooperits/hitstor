//go:build linux

package fs

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// maxXattrValue caps a single attribute value. Linux allows up to 64 KiB per
// attribute on most filesystems; anything larger is not system state we need
// for a bare-metal restore and would bloat the tree blob.
const maxXattrValue = 64 * 1024

// ReadXattrs returns the extended attributes of path, keyed by attribute name.
// Symlinks are read without being followed, matching the Lstat-based walk.
//
// A filesystem without xattr support yields an empty map rather than an error.
// Individual attributes that cannot be read (typically trusted.* without
// CAP_SYS_ADMIN) are skipped, since a partial set is more useful than none.
func ReadXattrs(path string) (map[string][]byte, error) {
	names, err := listXattrNames(path)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}

	attrs := make(map[string][]byte, len(names))
	for _, name := range names {
		value, err := getXattrValue(path, name)
		if err != nil {
			// Unreadable or removed between listing and reading: skip it.
			continue
		}
		attrs[name] = value
	}
	if len(attrs) == 0 {
		return nil, nil
	}
	return attrs, nil
}

// listXattrNames returns the attribute names set on path.
func listXattrNames(path string) ([]string, error) {
	size, err := unix.Llistxattr(path, nil)
	if err != nil {
		if unsupportedXattrErr(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("fs: list xattrs for %s: %w", path, err)
	}
	if size == 0 {
		return nil, nil
	}

	buf := make([]byte, size)
	size, err = unix.Llistxattr(path, buf)
	if err != nil {
		if unsupportedXattrErr(err) {
			return nil, nil
		}
		// The set shrank between calls; treat as no attributes.
		if errors.Is(err, unix.ERANGE) {
			return nil, nil
		}
		return nil, fmt.Errorf("fs: list xattrs for %s: %w", path, err)
	}

	return splitNullTerminated(buf[:size]), nil
}

// getXattrValue reads a single attribute value.
func getXattrValue(path, name string) ([]byte, error) {
	size, err := unix.Lgetxattr(path, name, nil)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return []byte{}, nil
	}
	if size > maxXattrValue {
		return nil, fmt.Errorf("fs: xattr %s on %s exceeds %d bytes", name, path, maxXattrValue)
	}

	buf := make([]byte, size)
	size, err = unix.Lgetxattr(path, name, buf)
	if err != nil {
		return nil, err
	}
	return buf[:size], nil
}

// WriteXattrs applies attrs to path, without following symlinks.
//
// It is best-effort by design: attributes the target filesystem or the current
// privilege level rejects are skipped so that a restore is never aborted by
// metadata that cannot be reproduced. Callers that need to report skipped
// attributes should use WriteXattrsReport.
func WriteXattrs(path string, attrs map[string][]byte) error {
	_, err := WriteXattrsReport(path, attrs)
	return err
}

// WriteXattrsReport applies attrs and returns the names that could not be set.
// The error is non-nil only for failures unrelated to a specific attribute.
func WriteXattrsReport(path string, attrs map[string][]byte) ([]string, error) {
	if len(attrs) == 0 {
		return nil, nil
	}

	var skipped []string
	for name, value := range attrs {
		if err := unix.Lsetxattr(path, name, value, 0); err != nil {
			skipped = append(skipped, name)
		}
	}
	return skipped, nil
}

// unsupportedXattrErr reports whether err means the filesystem has no xattr
// support, which is a normal condition rather than a failure.
func unsupportedXattrErr(err error) bool {
	return errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP)
}

// splitNullTerminated splits the NUL-separated name list returned by listxattr.
func splitNullTerminated(buf []byte) []string {
	var names []string
	start := 0
	for i, b := range buf {
		if b != 0 {
			continue
		}
		if i > start {
			names = append(names, string(buf[start:i]))
		}
		start = i + 1
	}
	if start < len(buf) {
		names = append(names, string(buf[start:]))
	}
	return names
}
