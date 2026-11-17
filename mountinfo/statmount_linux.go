package mountinfo

import (
	"errors"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// errStatmountUnsupported is returned when statmount(2) can't be used to fill
// in Info (because the kernel is too old, or the syscall is blocked by seccomp).
var errStatmountUnsupported = errors.New("statmount not supported")

const (
	// statmountMask is a set of statmount fields needed to fill in Info.
	statmountMask = unix.STATMOUNT_SB_BASIC | unix.STATMOUNT_MNT_BASIC |
		unix.STATMOUNT_PROPAGATE_FROM | unix.STATMOUNT_MNT_ROOT |
		unix.STATMOUNT_MNT_POINT | unix.STATMOUNT_FS_TYPE |
		unix.STATMOUNT_MNT_OPTS | unix.STATMOUNT_FS_SUBTYPE |
		unix.STATMOUNT_SB_SOURCE

	// statmountAlways is a subset of statmountMask that is always set in
	// the returned mask (if supported by the kernel). The rest of the bits
	// are only set if the corresponding string is not empty.
	statmountAlways = unix.STATMOUNT_SB_BASIC | unix.STATMOUNT_MNT_BASIC |
		unix.STATMOUNT_PROPAGATE_FROM | unix.STATMOUNT_MNT_ROOT |
		unix.STATMOUNT_FS_TYPE
)

// statmount returns Info for a mount with a given unique mount ID.
//
// It returns errStatmountUnsupported if the kernel does not support
// statmount, or does not support all the fields needed to fill in Info
// (which requires Linux 6.13+).
func statmount(id uint64) (*Info, error) {
	st, err := unix.Statmount(&unix.MntIdReq{
		Id:    id,
		Param: statmountMask | unix.STATMOUNT_SUPPORTED_MASK,
	}, 0)
	if err != nil {
		if isUnsupported(err) {
			return nil, errStatmountUnsupported
		}
		return nil, err
	}
	if !statmountSupported(st) {
		return nil, errStatmountUnsupported
	}
	return statmountToInfo(st), nil
}

// isUnsupported checks if err from a syscall means it is not supported
// (or blocked by seccomp).
func isUnsupported(err error) bool {
	return errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EPERM)
}

// statmountSupported checks whether st has all the information needed to fill
// in Info.
func statmountSupported(st *unix.Statmount_t) bool {
	if st.Mask&statmountAlways != statmountAlways {
		return false
	}
	// Linux 6.15+ tells which fields it supports.
	if st.Mask&unix.STATMOUNT_SUPPORTED_MASK != 0 {
		return st.Supported_mask&statmountMask == statmountMask
	}
	// For older kernels, the presence of STATMOUNT_SB_SOURCE (which is
	// non-empty for all practical purposes) means Linux 6.13+, which
	// also supports STATMOUNT_MNT_OPTS and STATMOUNT_FS_SUBTYPE.
	return st.Mask&unix.STATMOUNT_SB_SOURCE != 0
}

// statmountToInfo converts statmount(2) result to Info, so that all the
// fields are the same as if they were parsed from /proc/self/mountinfo
// (see show_mountinfo in the kernel's fs/proc_namespace.c).
func statmountToInfo(st *unix.Statmount_t) *Info {
	fsType := st.Fs_type
	if st.Fs_subtype != "" {
		fsType += "." + st.Fs_subtype
	}
	return &Info{
		ID:             int(st.Mnt_id_old),
		Parent:         int(st.Mnt_parent_id_old),
		Major:          int(st.Sb_dev_major),
		Minor:          int(st.Sb_dev_minor),
		Root:           st.Mnt_root,
		Mountpoint:     st.Mnt_point,
		Options:        mntAttrToOptions(st.Mnt_attr),
		Optional:       propagationToOptional(st),
		FSType:         fsType,
		Source:         st.Sb_source,
		VFSOptions:     sbOptions(st),
		UniqueID:       st.Mnt_id,
		ParentUniqueID: st.Mnt_parent_id,
	}
}

// mntAttrToOptions converts MOUNT_ATTR_* flags to per-mount options,
// in the same format as show_vfsmnt_opts in the kernel.
func mntAttrToOptions(attr uint64) string {
	var b strings.Builder
	if attr&unix.MOUNT_ATTR_RDONLY != 0 {
		b.WriteString("ro")
	} else {
		b.WriteString("rw")
	}
	for _, o := range []struct {
		flag uint64
		str  string
	}{
		{unix.MOUNT_ATTR_NOSUID, ",nosuid"},
		{unix.MOUNT_ATTR_NODEV, ",nodev"},
		{unix.MOUNT_ATTR_NOEXEC, ",noexec"},
		{unix.MOUNT_ATTR_NOATIME, ",noatime"},
		{unix.MOUNT_ATTR_NODIRATIME, ",nodiratime"},
	} {
		if attr&o.flag != 0 {
			b.WriteString(o.str)
		}
	}
	// MOUNT_ATTR_RELATIME is 0, so it has to be checked separately.
	if attr&unix.MOUNT_ATTR__ATIME == unix.MOUNT_ATTR_RELATIME {
		b.WriteString(",relatime")
	}
	if attr&unix.MOUNT_ATTR_NOSYMFOLLOW != 0 {
		b.WriteString(",nosymfollow")
	}
	if attr&unix.MOUNT_ATTR_IDMAP != 0 {
		b.WriteString(",idmapped")
	}
	return b.String()
}

// propagationToOptional returns mount propagation information in the same
// format as optional fields of /proc/self/mountinfo.
func propagationToOptional(st *unix.Statmount_t) string {
	var opts []string
	if st.Mnt_propagation&unix.MS_SHARED != 0 {
		opts = append(opts, "shared:"+strconv.FormatUint(st.Mnt_peer_group, 10))
	}
	if st.Mnt_propagation&unix.MS_SLAVE != 0 {
		opts = append(opts, "master:"+strconv.FormatUint(st.Mnt_master, 10))
		if pf := st.Propagate_from; pf != 0 && pf != st.Mnt_master {
			opts = append(opts, "propagate_from:"+strconv.FormatUint(pf, 10))
		}
	}
	if st.Mnt_propagation&unix.MS_UNBINDABLE != 0 {
		opts = append(opts, "unbindable")
	}
	return strings.Join(opts, " ")
}

// sbOptions returns superblock options in the same format as the last
// field of /proc/self/mountinfo.
func sbOptions(st *unix.Statmount_t) string {
	var b strings.Builder
	if st.Sb_flags&unix.MS_RDONLY != 0 {
		b.WriteString("ro")
	} else {
		b.WriteString("rw")
	}
	for _, o := range []struct {
		flag uint32
		str  string
	}{
		{unix.MS_SYNCHRONOUS, ",sync"},
		{unix.MS_DIRSYNC, ",dirsync"},
		{unix.MS_LAZYTIME, ",lazytime"},
	} {
		if st.Sb_flags&o.flag != 0 {
			b.WriteString(o.str)
		}
	}
	if st.Mnt_opts != "" {
		b.WriteString(",")
		b.WriteString(st.Mnt_opts)
	}
	return b.String()
}

// listmount returns unique IDs of all mounts in the subtree of a mount with
// a given unique ID (not including the mount itself). Use [unix.LSMT_ROOT] to
// list all mounts visible to the current process.
func listmount(id uint64) ([]uint64, error) {
	var ids []uint64
	buf := make([]uint64, 512)
	req := unix.MntIdReq{Id: id}
	for {
		n, err := unix.Listmount(&req, buf, 0)
		if err != nil {
			if isUnsupported(err) {
				return nil, errStatmountUnsupported
			}
			return nil, err
		}
		ids = append(ids, buf[:n]...)
		if n < len(buf) {
			return ids, nil
		}
		// Continue after the last returned ID.
		req.Param = buf[n-1]
	}
}

// getMountsStatmount is like parseMountTable, but uses listmount(2) and
// statmount(2) instead of parsing /proc/thread-self/mountinfo.
func getMountsStatmount(filter FilterFunc) ([]*Info, error) {
	// listmount and statmount work in the mount namespace of the current
	// thread, so make sure it does not change while we're at it.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ids, err := listmount(unix.LSMT_ROOT)
	if err != nil {
		return nil, err
	}
	out := []*Info{}
	for _, id := range ids {
		p, err := statmount(id)
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				// Unmounted since listmount.
				continue
			}
			return nil, err
		}
		var skip, stop bool
		if filter != nil {
			skip, stop = filter(p)
			if skip {
				continue
			}
		}
		out = append(out, p)
		if stop {
			break
		}
	}
	return out, nil
}
