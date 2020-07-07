// +build linux freebsd,cgo

package mountinfo

import (
	"path/filepath"

	"golang.org/x/sys/unix"
)

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

	entries, err := GetMounts(SingleEntryFilter(path))
	if err != nil {
		return false, err
	}

	return len(entries) > 0, nil
}
