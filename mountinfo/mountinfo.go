package mountinfo

import (
	"os"
	"strconv"
	"strings"
)

// GetMounts retrieves a list of mounts for the current running process,
// with an optional filter applied (use nil for no filter).
func GetMounts(f FilterFunc) ([]*Info, error) {
	return parseMountTable(f)
}

// Mounted determines if a specified path is a mount point. In case of any
// error, false (and an error) is returned.
//
// If a non-existent path is specified, an appropriate error is returned.
// In case the caller is not interested in this particular error, it should
// be handled separately using e.g. errors.Is(err, fs.ErrNotExist).
func Mounted(path string) (bool, error) {
	// root is always mounted
	if path == string(os.PathSeparator) {
		return true, nil
	}
	return mounted(path)
}

// MountOf returns information about the mount which path belongs to (i.e.
// the mount containing the file or directory the path refers to, after
// resolving all symlinks). If several mounts are stacked on the same mount
// point, the topmost (i.e. visible) one is returned.
//
// This is a more efficient and correct alternative to calling GetMounts with
// ParentsFilter and choosing an entry with the longest mount point. On Linux
// 6.13+, it uses statx(2) and statmount(2) and avoids reading mountinfo.
//
// If a non-existent path is specified, an appropriate error is returned,
// which can be checked using errors.Is(err, fs.ErrNotExist).
//
// This function is not implemented on Windows.
func MountOf(path string) (*Info, error) {
	return mountOf(path)
}

// Info reveals information about a particular mounted filesystem. This
// struct is populated from the content in the /proc/<pid>/mountinfo file,
// or using statmount(2).
type Info struct {
	// ID is a unique identifier of the mount (may be reused after umount).
	ID int

	// Parent is the ID of the parent mount (or of self for the root
	// of this mount namespace's mount tree).
	Parent int

	// Major and Minor are the major and the minor components of the Dev
	// field of unix.Stat_t structure returned by unix.*Stat calls for
	// files on this filesystem.
	Major, Minor int

	// Root is the pathname of the directory in the filesystem which forms
	// the root of this mount.
	Root string

	// Mountpoint is the pathname of the mount point relative to the
	// process's root directory.
	Mountpoint string

	// Options is a comma-separated list of mount options.
	Options string

	// Optional are zero or more fields of the form "tag[:value]",
	// separated by a space.  Currently, the possible optional fields are
	// "shared", "master", "propagate_from", and "unbindable". For more
	// information, see mount_namespaces(7) Linux man page.
	Optional string

	// FSType is the filesystem type in the form "type[.subtype]".
	FSType string

	// Source is filesystem-specific information, or "none".
	Source string

	// VFSOptions is a comma-separated list of superblock options.
	VFSOptions string

	// UniqueID is a unique identifier of the mount (never reused), as
	// returned by statmount(2) and statx(2) with STATX_MNT_ID_UNIQUE
	// (Linux 6.8+). Note that it differs from ID.
	//
	// It is 0 when not available (e.g. when Info is parsed from mountinfo).
	UniqueID uint64

	// ParentUniqueID is a unique identifier of the parent mount (or of
	// self for the root of this mount namespace's mount tree).
	//
	// It is 0 when not available (e.g. when Info is parsed from mountinfo).
	ParentUniqueID uint64
}

// optionalField returns the value of a tag from the Optional field
// (e.g. "1" for tag "shared" if Optional is "shared:1 master:2"),
// and whether the tag is present.
func (i *Info) optionalField(tag string) (string, bool) {
	for f := range strings.FieldsSeq(i.Optional) {
		name, val, _ := strings.Cut(f, ":")
		if name == tag {
			return val, true
		}
	}
	return "", false
}

// optionalID is like optionalField, but parses the value as an integer ID.
func (i *Info) optionalID(tag string) (int, bool) {
	val, ok := i.optionalField(tag)
	if !ok {
		return 0, false
	}
	id, err := strconv.Atoi(val)
	if err != nil {
		return 0, false
	}
	return id, true
}

// PeerGroup returns the ID of the peer group, and true, if the mount is
// shared (i.e. its Optional field contains "shared:ID"). Otherwise, it
// returns 0 and false.
//
// For more information on mount propagation, see mount_namespaces(7).
func (i *Info) PeerGroup() (int, bool) {
	return i.optionalID("shared")
}

// Master returns the ID of the peer group from which the mount receives
// propagation events, and true, if the mount is a slave (i.e. its Optional
// field contains "master:ID"). Otherwise, it returns 0 and false.
//
// Note that a mount can be both a slave and shared, in which case both
// PeerGroup and Master return true.
func (i *Info) Master() (int, bool) {
	return i.optionalID("master")
}

// PropagateFrom returns the ID of the closest dominant peer group under the
// process's root directory, and true, if it differs from the one returned
// by Master (i.e. its Optional field contains "propagate_from:ID").
// Otherwise, it returns 0 and false.
func (i *Info) PropagateFrom() (int, bool) {
	return i.optionalID("propagate_from")
}

// Unbindable reports whether the mount is unbindable.
func (i *Info) Unbindable() bool {
	_, ok := i.optionalField("unbindable")
	return ok
}
