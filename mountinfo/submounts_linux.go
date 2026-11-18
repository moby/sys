package mountinfo

import (
	"errors"
	"os"
	"runtime"
	"slices"

	"golang.org/x/sys/unix"
)

func submounts(path string) ([]*Info, error) {
	// Make sure the mount namespace does not change in between the calls.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Use AT_NO_AUTOMOUNT to be consistent with Mounted and MountOf.
	var st unix.Statx_t
	err := unix.Statx(unix.AT_FDCWD, path, unix.AT_NO_AUTOMOUNT, unix.STATX_MNT_ID_UNIQUE, &st)
	if err != nil {
		if !isUnsupported(err) {
			return nil, &os.PathError{Op: "statx", Path: path, Err: err}
		}
		return submountsByPrefixSlow(path)
	}

	// The fast path only works if path is a mount point. Otherwise, the
	// subtree to look at is that of the parent mount (possibly all
	// mounts), and parsing mountinfo is faster.
	if st.Mask&unix.STATX_MNT_ID_UNIQUE != 0 &&
		st.Attributes_mask&unix.STATX_ATTR_MOUNT_ROOT != 0 &&
		st.Attributes&unix.STATX_ATTR_MOUNT_ROOT != 0 {
		mounts, err := submountsStatmount(st.Mnt_id)
		if !errors.Is(err, errStatmountUnsupported) {
			return mounts, err
		}
	}

	return submountsByPrefixSlow(path)
}

// submountsStatmount returns the mount with a given unique ID, the mounts
// stacked under it on the same mount point, and all their submounts,
// sorted by unique mount ID.
func submountsStatmount(id uint64) ([]*Info, error) {
	top, err := statmount(id)
	if err != nil {
		return nil, err
	}

	// Find the bottommost of the mounts stacked on the same mount point,
	// as its subtree includes all the mounts on top of it, as well as
	// those hidden by them.
	out := []*Info{top}
	bottom := top
	for bottom.ParentUniqueID != bottom.UniqueID {
		parent, err := statmount(bottom.ParentUniqueID)
		if err != nil || parent.Mountpoint != top.Mountpoint {
			// Note an error here is not fatal, as the parent
			// mount may be inaccessible (e.g. outside of chroot).
			break
		}
		out = append(out, parent)
		bottom = parent
	}

	ids, err := listmount(bottom.UniqueID)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if slices.ContainsFunc(out, func(i *Info) bool { return i.UniqueID == id }) {
			// Already have it (one of the stacked mounts).
			continue
		}
		info, err := statmount(id)
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				// Unmounted since listmount.
				continue
			}
			return nil, err
		}
		out = append(out, info)
	}

	// This is the same order as in mountinfo (since Linux 6.8).
	slices.SortFunc(out, func(a, b *Info) int {
		return cmpUint64(a.UniqueID, b.UniqueID)
	})
	return out, nil
}

func cmpUint64(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
