//go:build freebsd || darwin
// +build freebsd darwin

package mountinfo

import "golang.org/x/sys/unix"

func getMountinfo(entry *unix.Statfs_t) *Info {
	var mountinfo Info
	mountinfo.Mountpoint = unix.ByteSliceToString(entry.Mntonname[:])
	mountinfo.FSType = unix.ByteSliceToString(entry.Fstypename[:])
	mountinfo.Source = unix.ByteSliceToString(entry.Mntfromname[:])
	return &mountinfo
}
