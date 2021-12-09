package mountinfo

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// MountedFast is a method of detecting mount points without reading
// mountinfo from procfs. A mount point check is guaranteed to be a
// mount point or not only when sure is true. When sure is false, the
// caller needs to check for other methods (eg: parse /proc/mounts)
// to successfully determine if it is a mount point. This function
// is only available on Linux.
func MountedFast(path string) (mounted, sure bool, err error) {
	// root is always mounted
	if path == string(os.PathSeparator) {
		return true, true, nil
	}

	path, err = normalizePath(path)
	if err != nil {
		return false, false, err
	}

	return mountedFast(path)
}

// mountedFast combines mountedByOpenAt2 and mountedByStat.
// A mount point check is guaranteed to be a mount point or not only
// when sure is true. When sure is false, the caller needs to check
// for other methods (eg: parse /proc/mounts) to successfully
// determine if it is a mount point.
func mountedFast(normalizedPath string) (mounted, sure bool, err error) {
	// Try a fast path, using openat2() with RESOLVE_NO_XDEV.
	mounted, err = mountedByOpenat2(normalizedPath)
	if err == nil {
		return mounted, true, nil
	}

	// Another fast path: compare st.st_dev fields.
	mounted, err = mountedByStat(normalizedPath)
	// This does not work for bind mounts, so false negative
	// is possible, therefore only trust if return is true.
	if mounted && err == nil {
		return mounted, true, nil
	}

	// NB: not sure
	return false, false, err
}

// mountedByOpenat2 is a method of detecting a mount that works for all kinds
// of mounts (incl. bind mounts), but requires a recent (v5.6+) linux kernel.
func mountedByOpenat2(path string) (bool, error) {
	dir, last := filepath.Split(path)

	dirfd, err := unix.Openat2(unix.AT_FDCWD, dir, &unix.OpenHow{
		Flags: unix.O_PATH | unix.O_CLOEXEC,
	})
	if err != nil {
		return false, &os.PathError{Op: "openat2", Path: dir, Err: err}
	}
	fd, err := unix.Openat2(dirfd, last, &unix.OpenHow{
		Flags:   unix.O_PATH | unix.O_CLOEXEC | unix.O_NOFOLLOW,
		Resolve: unix.RESOLVE_NO_XDEV,
	})
	_ = unix.Close(dirfd)
	switch err { //nolint:errorlint // unix errors are bare
	case nil: // definitely not a mount
		_ = unix.Close(fd)
		return false, nil
	case unix.EXDEV: // definitely a mount
		return true, nil
	}
	// not sure
	return false, &os.PathError{Op: "openat2", Path: path, Err: err}
}

func mounted(path string) (bool, error) {
	path, err := normalizePath(path)
	if err != nil {
		return false, err
	}

	// Try all fast paths.
	mounted, sure, err := mountedFast(path)
	if sure && err == nil {
		return mounted, nil
	}

	// Fallback to parsing mountinfo
	return mountedByMountinfo(path)
}
