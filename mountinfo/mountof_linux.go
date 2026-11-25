package mountinfo

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func mountOf(path string) (*Info, error) {
	// Make sure the mount namespace does not change in between the calls.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Use AT_NO_AUTOMOUNT to be consistent with Mounted.
	const flags = unix.AT_NO_AUTOMOUNT
	var st unix.Statx_t
	err := unix.Statx(unix.AT_FDCWD, path, flags, unix.STATX_MNT_ID_UNIQUE, &st)
	if err != nil {
		if !isUnsupported(err) {
			return nil, &os.PathError{Op: "statx", Path: path, Err: err}
		}
		// No statx (or blocked by seccomp).
		return mountOfByParentsSlow(path)
	}

	// Fast path: statx + statmount (Linux 6.8+, all Info fields need 6.13+).
	if st.Mask&unix.STATX_MNT_ID_UNIQUE != 0 {
		info, err := statmount(st.Mnt_id)
		if !errors.Is(err, errStatmountUnsupported) {
			return info, err
		}
		// Get the old (non-unique) mount ID which is used in mountinfo.
		err = unix.Statx(unix.AT_FDCWD, path, flags, unix.STATX_MNT_ID, &st)
		if err != nil {
			return nil, &os.PathError{Op: "statx", Path: path, Err: err}
		}
	}

	// Slower path: statx + mountinfo by mount ID (Linux 5.8+).
	if st.Mask&unix.STATX_MNT_ID != 0 {
		return mountOfByID(int(st.Mnt_id))
	}

	// Slowest path.
	return mountOfByParentsSlow(path)
}

// mountOfByID returns Info of a mount with a given (non-unique) mount ID.
func mountOfByID(id int) (*Info, error) {
	mounts, err := GetMounts(func(m *Info) (skip, stop bool) {
		return m.ID != id, m.ID == id
	})
	if err != nil {
		return nil, err
	}
	if len(mounts) == 0 {
		// Unmounted since statx.
		return nil, fmt.Errorf("mount ID %d: %w", id, unix.ENOENT)
	}
	return mounts[0], nil
}
