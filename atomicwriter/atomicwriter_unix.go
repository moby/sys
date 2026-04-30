//go:build !windows

package atomicwriter

import "os"

func atomicwriterRenameAt(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
