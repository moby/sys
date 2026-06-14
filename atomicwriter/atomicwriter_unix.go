//go:build !windows

package atomicwriter

import "os"

func atomicwriterRename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
