package mountinfo

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

func int8SliceToString(is []int8) string {
	for i := range is {
		if is[i] == 0 {
			is = is[:i]
			break
		}
	}
	return *(*string)(unsafe.Pointer(&is))
}

func getMountinfo(entry *unix.Statfs_t) *Info {
	var mountinfo Info
	mountinfo.Mountpoint = int8SliceToString(entry.F_mntonname[:])
	mountinfo.FSType = int8SliceToString(entry.F_fstypename[:])
	mountinfo.Source = int8SliceToString(entry.F_mntfromname[:])
	return &mountinfo
}
