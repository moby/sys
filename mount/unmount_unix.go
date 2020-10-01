// +build !windows

package mount

import (
	"os/exec"

	"golang.org/x/sys/unix"
)

func unmountBare(target string, flags int) error {
	return unix.Unmount(target, flags)
}

// fuseSuperMagic is defined in statfs(2)
const fuseSuperMagic = 0x65735546

func isFUSE(dir string) bool {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return false
	}
	return st.Type == fuseSuperMagic
}

// unmountFUSE attempts to unmount using fusermount/fusermount3 helper binary.
//
// For FUSE mounts, using these helper binaries is preferred, see:
// https://github.com/containerd/containerd/pull/3765#discussion_r342083514
func unmountFUSE(target string) error {
	var err error
	for _, helperBinary := range []string{"fusermount3", "fusermount"} {
		cmd := exec.Command(helperBinary, "-u", target)
		err = cmd.Run()
		if err == nil {
			return nil
		}
	}
	return err
}

func unmount(target string, flags int) error {
	if isFUSE(target) {
		if err := unmountFUSE(target); err == nil {
			return nil
		}
	}
	err := unmountBare(target, flags)
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
