//go:build freebsd || openbsd || darwin
// +build freebsd openbsd darwin

package mountinfo

import (
	"reflect"
	"syscall"
)

func getInfo(r reflect.Value, a string, b string) string {
	if r.FieldByName(a) != (reflect.Value{}) {
		r = r.FieldByName(a)
	} else {
		r = r.FieldByName(b)
	}
	var bs []byte
	for i := 0; i < r.Len(); i++ {
		i8 := r.Index(i).Int()
		if i8 == 0 {
			break
		}
		bs = append(bs, byte(i8))
	}
	return string(bs)
}

// parseMountTable returns information about mounted filesystems
func parseMountTable(filter FilterFunc) ([]*Info, error) {
	count, err := syscall.Getfsstat(nil, 1 /* MNT_WAIT */)
	if err != nil {
		return nil, err
	}

	entries := make([]syscall.Statfs_t, count)
	_, err = syscall.Getfsstat(entries, 1 /* MNT_WAIT */)
	if err != nil {
		return nil, err
	}

	var out []*Info
	for _, entry := range entries {
		var mountinfo Info
		var skip, stop bool
		r := reflect.ValueOf(entry)
		mountinfo.Mountpoint = getInfo(r, "Mntonname", "F_mntonname" /* OpenBSD */)
		mountinfo.FSType = getInfo(r, "Fstypename", "F_fstypename" /* OpenBSD */)
		mountinfo.Source = getInfo(r, "Mntfromname", "F_mntfromname" /* OpenBSD */)

		if filter != nil {
			// filter out entries we're not interested in
			skip, stop = filter(&mountinfo)
			if skip {
				continue
			}
		}

		out = append(out, &mountinfo)
		if stop {
			break
		}
	}
	return out, nil
}

func mounted(path string) (bool, error) {
	path, err := normalizePath(path)
	if err != nil {
		return false, err
	}
	// Fast path: compare st.st_dev fields.
	// This should always work for FreeBSD and OpenBSD.
	mounted, err := mountedByStat(path)
	if err == nil {
		return mounted, nil
	}

	// Fallback to parsing mountinfo
	return mountedByMountinfo(path)
}
