// +build !windows

package mount

import (
	"fmt"
	"sort"

	"github.com/moby/sys/mountinfo"
	"golang.org/x/sys/unix"
)

func recursiveUnmount(target string) error {
	// Fast path, works if target is a mount point that can be unmounted.
	// On Linux, mntDetach flag ensures a recursive unmount.  For other
	// platforms, if there are submounts, we'll get EBUSY (and fall back
	// to the slow path). NOTE we do not ignore EINVAL here as target might
	// not be a mount point itself (but there can be mounts underneath).
	if err := unix.Unmount(target, mntDetach); err == nil {
		return nil
	}

	// Slow path: get all submounts, sort, unmount one by one.
	mounts, err := mountinfo.GetMounts(mountinfo.PrefixFilter(target))
	if err != nil {
		return err
	}

	// Make the deepest mount be first
	sort.Slice(mounts, func(i, j int) bool {
		return len(mounts[i].Mountpoint) > len(mounts[j].Mountpoint)
	})

	var suberr error
	for i, m := range mounts {
		err = unmount(m.Mountpoint, mntDetach)
		if err != nil {
			if i == len(mounts)-1 { // last mount
				return fmt.Errorf("%w (possible cause: %s)", err, suberr)
			}
			// This is a submount, we can ignore the error for now,
			// the final unmount will fail if this is a real problem.
			// With that in mind, the _first_ failed unmount error
			// might be the real error cause, so let's keep it.
			if suberr == nil {
				suberr = err
			}
		}
	}
	return nil
}

func unmount(target string, flags int) error {
	err := unix.Unmount(target, flags)
	if err == nil || err == unix.EINVAL {
		// Ignore "not mounted" error here. Note the same error
		// can be returned if flags are invalid, so this code
		// assumes that the flags value is always correct.
		return nil
	}

	return &mountError{
		op:     "umount",
		target: target,
		flags:  uintptr(flags),
		err:    err,
	}
}
