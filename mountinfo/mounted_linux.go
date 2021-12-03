package mountinfo

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// MountedBind is a Linux-specific function, similar to Mounted, but with
// additional workarounds to decrease the possibility of misdetecting
// a bind mount.
//
// On Linux (before kernel 5.8) bind mount detection is not reliable (false
// negatives are possible) on systems that perform lots of mounts and unmounts,
// due to a kernel issue. This function tries to mitigate that issue by
// re-reading mountinfo, decreasing the possibility of a false negative.
//
// This function increases system load. It should only be used for bind
// mounts, if a user wants to sacrifice system performance for higher reliability.
func MountedBind(path string) (bool, error) {
	return mountedWithFallback(path, mountedByMountinfoWithRetries)
}

// mountedByMountinfoWithRetries is like mountedByMountinfo, with
// the addition of a workaround for a kernel bug (fixed in Linux 5.8).
// This kernel bug is essentially a race between reading mountinfo
// and performing an unmount. In the case next to be read mount entry is
// removed from the kernel data structures, it prematurely returns EOF.
// As a result, sometimes only a half of mountinfo is read, leading to
// inability to find a legitimate existing mount.
//
// Alas, there is no good solution to that problem. A workaround, implemented
// here, is to re-read mountinfo again if the mount is not found. With multiple
// re-reads, the chances of repeatedly hitting the race are decreasing.
//
// Those re-reads are costly, so there is an additional optimization -- we
// assume that if two subsequent reads returned the same number of entries,
// there is no issue, and thus there is no need to re-read mountinfo again.
// This is not a great heuristic but in case there are no parallel
// mounts/unmounts it limits the number of reads to 2.
//
// Note that the main idea here is not to get two consecutive consistent
// readings (with the same number of entries), but rather a complete reading.
// Since there is no way to figure out if the reading is complete, we compare
// the number of entries between the two readings.
//
// Finally, in case of massively parallel mounts/unmounts the number of entries
// will always be different, and in this case maxTries is the main mechanism to
// exit the re-reading loop.
func mountedByMountinfoWithRetries(path string) (bool, error) {
	var total, oldTotal, try int
	const maxTries = 3

again:
	// The filter is the same as SingleEntryFilter,
	// with the addition of counting total entries.
	entries, err := GetMounts(func(m *Info) (bool, bool) {
		if m.Mountpoint == path {
			return false, true // don't skip, stop now
		}
		total++
		return true, false // skip, keep going
	})
	if err != nil {
		return false, err
	}
	if len(entries) > 0 {
		return true, nil
	}

	try++
	if try == maxTries {
		return false, nil
	} else if total == oldTotal {
		// Two last reads resulted in the same number of entries,
		// which most probably means we get a complete reading.
		return false, nil
	} else {
		oldTotal = total
		total = 0
		goto again
	}
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

// mountedWithFallback tries all the fast approaches to figure out whether path
// is a mount point. It calls fallbackFn if all those approarches failed.
func mountedWithFallback(path string, fallbackFn func(string) (bool, error)) (bool, error) {
	path, err := normalizePath(path)
	if err != nil {
		return false, err
	}
	// Try a fast path, using openat2() with RESOLVE_NO_XDEV.
	mounted, err := mountedByOpenat2(path)
	if err == nil {
		return mounted, nil
	}
	// Another fast path: compare st.st_dev fields.
	mounted, err = mountedByStat(path)
	// This does not work for bind mounts, so false negative
	// is possible, therefore only trust if return is true.
	if mounted && err == nil {
		return mounted, nil
	}

	// Fallback to parsing mountinfo
	return fallbackFn(path)
}

func mounted(path string) (bool, error) {
	return mountedWithFallback(path, mountedByMountinfo)
}
