// +build linux freebsd,cgo

package mountinfo

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func normalizePath(path string) (realPath string, err error) {
	if realPath, err = filepath.Abs(path); err != nil {
		return "", fmt.Errorf("unable to get absolute path for %q: %w", path, err)
	}
	if realPath, err = filepath.EvalSymlinks(realPath); err != nil {
		return "", fmt.Errorf("failed to canonicalise path for %q: %w", path, err)
	}
	if _, err := os.Stat(realPath); err != nil {
		return "", fmt.Errorf("failed to stat target %q of %q: %w", realPath, path, err)
	}
	return realPath, nil
}

func mounted(path string) (bool, error) {
	var st unix.Stat_t

	err := unix.Lstat(path, &st)
	switch err {
	case unix.ENOENT:
		// Nonexistent path, so not a mount point.
		return false, nil
	case nil:
		dev := st.Dev
		err = unix.Lstat(filepath.Dir(path), &st)
		if err == nil && dev != st.Dev {
			// Device number differs from that of parent,
			// so definitely a mount point.
			return true, nil
		}
	}

	path, err = normalizePath(path)
	if err != nil {
		return false, err
	}

	entries, err := GetMounts(SingleEntryFilter(path))
	if err != nil {
		return false, err
	}

	return len(entries) > 0, nil
}
