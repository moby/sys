package user

import "os"

// mkdirAs creates path, optionally creating any missing parent directories.
//
// On Windows this is currently a thin wrapper around os.Mkdir and
// os.MkdirAll. Unlike the Unix implementation, ownership and permission
// bits are not applied, and errors for existing paths follow the underlying
// os package semantics (for example, an existing non-directory does not
// return ENOTDIR).
func mkdirAs(path string, _ os.FileMode, _, _ int, mkAll, _ bool) error {
	if mkAll {
		return os.MkdirAll(path, 0)
	}
	return os.Mkdir(path, 0)
}
