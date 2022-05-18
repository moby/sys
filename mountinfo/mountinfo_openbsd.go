package mountinfo

import "golang.org/x/sys/unix"

func int8SliceToString(is []int8) string {
	var bs []byte
	for _, i := range is {
		if i == 0 {
			break
		}
		bs = append(bs, byte(i))
	}
	return string(bs)
}

func getMountinfo(entry *unix.Statfs_t) *Info {
	var mountinfo Info
	mountinfo.Mountpoint = int8SliceToString(entry.F_mntonname[:])
	mountinfo.FSType = int8SliceToString(entry.F_fstypename[:])
	mountinfo.Source = int8SliceToString(entry.F_mntfromname[:])
	return &mountinfo
}
