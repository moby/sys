//go:build linux || freebsd || openbsd || darwin

package mountinfo

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func mountedByStat(path string) (bool, error) {
	var st unix.Stat_t

	if err := unix.Lstat(path, &st); err != nil {
		return false, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	dev := st.Dev
	parent := filepath.Dir(path)
	if err := unix.Lstat(parent, &st); err != nil {
		return false, &os.PathError{Op: "stat", Path: parent, Err: err}
	}
	if dev != st.Dev {
		// Device differs from that of parent,
		// so definitely a mount point.
		return true, nil
	}
	// NB: this does not detect bind mounts on Linux.
	return false, nil
}

func normalizePath(path string) (realPath string, err error) {
	if realPath, err = filepath.Abs(path); err != nil {
		return "", err
	}
	if realPath, err = filepath.EvalSymlinks(realPath); err != nil {
		return "", err
	}
	if _, err := os.Stat(realPath); err != nil {
		return "", err
	}
	return realPath, nil
}

func mountedByMountinfo(path string) (bool, error) {
	entries, err := GetMounts(SingleEntryFilter(path))
	if err != nil {
		return false, err
	}

	return len(entries) > 0, nil
}

// mountOfByParentsSlow is like mountOfByParents, but does not require path
// to be normalized.
func mountOfByParentsSlow(path string) (*Info, error) {
	path, err := normalizePath(path)
	if err != nil {
		return nil, err
	}
	return mountOfByParents(path)
}

// mountOfByParents finds the mount which path belongs to, by looking for the
// longest mount point which is a parent of path. The path must be normalized.
func mountOfByParents(path string) (*Info, error) {
	mounts, err := GetMounts(ParentsFilter(path))
	if err != nil {
		return nil, err
	}
	var res *Info
	for _, m := range mounts {
		// Several mounts can be stacked on the same mount point, and the
		// last one is the topmost one (well, unless MOVE_MOUNT_BENEATH was
		// used, which can only be figured out by statx + mount ID).
		if res == nil || len(m.Mountpoint) >= len(res.Mountpoint) {
			res = m
		}
	}
	if res == nil {
		// Should not happen, as / is always mounted.
		return nil, fmt.Errorf("no mount found for %q", path)
	}
	return res, nil
}
