package user

import (
	"fmt"
	"os"
	"path/filepath"
)

// FS is the filesystem contract used by the Mkdir*AndChownFS helpers.
type FS interface {
	Stat(name string) (os.FileInfo, error)
	Mkdir(name string, perm os.FileMode) error
	MkdirAll(name string, perm os.FileMode) error
	Chmod(name string, mode os.FileMode) error
	Chown(name string, uid, gid int) error
}

var (
	_ FS = &os.Root{}
	_ FS = &hostFS{}
)

// MkdirOpt is a type for options to pass to Mkdir calls
type MkdirOpt func(*mkdirOptions)

type mkdirOptions struct {
	onlyNew bool
}

// WithOnlyNew is an option for MkdirAllAndChown that will only change ownership and permissions
// on newly created directories.  If the directory already exists, it will not be modified
func WithOnlyNew(o *mkdirOptions) {
	o.onlyNew = true
}

// MkdirAllAndChown creates a directory (include any along the path) and then modifies
// ownership to the requested uid/gid.  By default, if the directory already exists, this
// function will still change ownership and permissions. If WithOnlyNew is passed as an
// option, then only the newly created directories will have ownership and permissions changed.
func MkdirAllAndChown(path string, mode os.FileMode, uid, gid int, opts ...MkdirOpt) error {
	return MkdirAllAndChownFS(nil, path, mode, uid, gid, opts...)
}

// MkdirAllAndChownFS creates a directory (including any along the path) on the
// provided filesystem and then modifies ownership to the requested uid/gid. If
// fsys is nil, the host filesystem is used.
func MkdirAllAndChownFS(fsys FS, path string, mode os.FileMode, uid, gid int, opts ...MkdirOpt) error {
	if fsys == nil {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		fsys = &hostFS{}
		path = absPath
	}

	options := mkdirOpts(opts)
	return mkdirAs(fsys, path, mode, uid, gid, true, options.onlyNew)
}

// MkdirAndChown creates a directory and then modifies ownership to the requested uid/gid.
// By default, if the directory already exists, this function still changes ownership and permissions.
// If WithOnlyNew is passed as an option, then only the newly created directory will have ownership
// and permissions changed.
// Note that unlike os.Mkdir(), this function does not return IsExist error
// in case path already exists.
func MkdirAndChown(path string, mode os.FileMode, uid, gid int, opts ...MkdirOpt) error {
	return MkdirAndChownFS(nil, path, mode, uid, gid, opts...)
}

// MkdirAndChownFS creates a directory on the provided filesystem and then
// modifies ownership to the requested uid/gid. If fsys is nil, the host
// filesystem is used.
func MkdirAndChownFS(fsys FS, path string, mode os.FileMode, uid, gid int, opts ...MkdirOpt) error {
	if fsys == nil {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		fsys = &hostFS{}
		path = absPath
	}

	options := mkdirOpts(opts)
	return mkdirAs(fsys, path, mode, uid, gid, false, options.onlyNew)
}

func mkdirOpts(opts []MkdirOpt) mkdirOptions {
	var options mkdirOptions
	for _, opt := range opts {
		opt(&options)
	}
	return options
}

// getRootUIDGID retrieves the remapped root uid/gid pair from the set of maps.
// If the maps are empty, then the root uid/gid will default to "real" 0/0
func getRootUIDGID(uidMap, gidMap []IDMap) (int, int, error) {
	uid, err := toHost(0, uidMap)
	if err != nil {
		return -1, -1, err
	}
	gid, err := toHost(0, gidMap)
	if err != nil {
		return -1, -1, err
	}
	return uid, gid, nil
}

// toContainer takes an id mapping, and uses it to translate a
// host ID to the remapped ID. If no map is provided, then the translation
// assumes a 1-to-1 mapping and returns the passed in id
func toContainer(hostID int, idMap []IDMap) (int, error) {
	if idMap == nil {
		return hostID, nil
	}
	for _, m := range idMap {
		if (int64(hostID) >= m.ParentID) && (int64(hostID) <= (m.ParentID + m.Count - 1)) {
			contID := int(m.ID + (int64(hostID) - m.ParentID))
			return contID, nil
		}
	}
	return -1, fmt.Errorf("host ID %d cannot be mapped to a container ID", hostID)
}

// toHost takes an id mapping and a remapped ID, and translates the
// ID to the mapped host ID. If no map is provided, then the translation
// assumes a 1-to-1 mapping and returns the passed in id #
func toHost(contID int, idMap []IDMap) (int, error) {
	if idMap == nil {
		return contID, nil
	}
	for _, m := range idMap {
		if (int64(contID) >= m.ID) && (int64(contID) <= (m.ID + m.Count - 1)) {
			hostID := int(m.ParentID + (int64(contID) - m.ID))
			return hostID, nil
		}
	}
	return -1, fmt.Errorf("container ID %d cannot be mapped to a host ID", contID)
}

// IdentityMapping contains a mappings of UIDs and GIDs.
// The zero value represents an empty mapping.
type IdentityMapping struct {
	UIDMaps []IDMap `json:"UIDMaps"`
	GIDMaps []IDMap `json:"GIDMaps"`
}

// RootPair returns a uid and gid pair for the root user. The error is ignored
// because a root user always exists, and the defaults are correct when the uid
// and gid maps are empty.
func (i IdentityMapping) RootPair() (int, int) {
	uid, gid, _ := getRootUIDGID(i.UIDMaps, i.GIDMaps)
	return uid, gid
}

// ToHost returns the host UID and GID for the container uid, gid.
// Remapping is only performed if the ids aren't already the remapped root ids
func (i IdentityMapping) ToHost(uid, gid int) (int, int, error) {
	var err error
	ruid, rgid := i.RootPair()

	if uid != ruid {
		ruid, err = toHost(uid, i.UIDMaps)
		if err != nil {
			return ruid, rgid, err
		}
	}

	if gid != rgid {
		rgid, err = toHost(gid, i.GIDMaps)
	}
	return ruid, rgid, err
}

// ToContainer returns the container UID and GID for the host uid and gid
func (i IdentityMapping) ToContainer(uid, gid int) (int, int, error) {
	ruid, err := toContainer(uid, i.UIDMaps)
	if err != nil {
		return -1, -1, err
	}
	rgid, err := toContainer(gid, i.GIDMaps)
	return ruid, rgid, err
}

// Empty returns true if there are no id mappings
func (i IdentityMapping) Empty() bool {
	return len(i.UIDMaps) == 0 && len(i.GIDMaps) == 0
}

type hostFS struct{}

func (*hostFS) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (*hostFS) Mkdir(name string, perm os.FileMode) error {
	return os.Mkdir(name, perm)
}

func (*hostFS) MkdirAll(name string, perm os.FileMode) error {
	return os.MkdirAll(name, perm)
}

func (*hostFS) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

func (*hostFS) Chown(name string, uid, gid int) error {
	return os.Chown(name, uid, gid)
}
