package mountinfo

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// tMount is a wrapper for unix.Mount which is used to prepare test cases.
// It skips the test case if mounting is not possible (i.e. user is not root),
// adds more context to mount error, if any, and installs a cleanup handler to
// undo the mount.
func tMount(t *testing.T, src, dst, fstype string, flags uintptr, options string) error {
	if os.Getuid() != 0 {
		t.Skip("root required for mounting")
	}

	err := unix.Mount(src, dst, fstype, flags, options)
	if err != nil {
		return &os.PathError{Path: dst, Op: "mount", Err: err}
	}
	t.Cleanup(func() {
		if err := unix.Unmount(dst, unix.MNT_DETACH); err != nil {
			t.Errorf("cleanup: unmount %q failed: %v", dst, err)
		}
	})
	return nil
}

var testMounts = []struct {
	desc       string
	isNotExist bool
	isMount    bool
	isBind     bool
	// prepare returns a path that needs to be checked, and the error, if any.
	//
	// It is responsible for cleanup (by using t.Cleanup).
	//
	// It should not fail the test (i.e. no calls to t.Error/t.Fatal).
	// The only exception to this rule is some cases use t.TempDir() for
	// simplicity (no need to check for errors or call t.Cleanup()), and
	// it may call t.Fatal, but practically we don't expect it.
	prepare func(t *testing.T) (string, error)
}{
	{
		desc:       "non-existent path",
		isNotExist: true,
		prepare: func(t *testing.T) (string, error) {
			return "/non/existent/path", nil
		},
	},
	{
		desc: "not mounted directory",
		prepare: func(t *testing.T) (dir string, err error) {
			dir = t.TempDir()
			return dir, err
		},
	},
	{
		desc:    "tmpfs mount",
		isMount: true,
		prepare: func(t *testing.T) (mnt string, err error) {
			mnt = t.TempDir()
			err = tMount(t, "tmpfs", mnt, "tmpfs", 0, "")
			return mnt, err
		},
	},
	{
		desc:    "tmpfs mount ending with a slash",
		isMount: true,
		prepare: func(t *testing.T) (mnt string, err error) {
			mnt = t.TempDir() + "/"
			err = tMount(t, "tmpfs", mnt, "tmpfs", 0, "")
			return mnt, err
		},
	},
	{
		desc:       "broken symlink",
		isNotExist: true,
		prepare: func(t *testing.T) (link string, err error) {
			dir := t.TempDir()
			link = filepath.Join(dir, "broken-symlink")
			err = os.Symlink("/some/non/existent/dest", link)
			return link, err
		},
	},
	{
		desc: "symlink to not mounted directory",
		prepare: func(t *testing.T) (link string, err error) {
			tmp := t.TempDir()

			dir, err := os.MkdirTemp(tmp, "dir")
			if err != nil {
				return
			}

			link = filepath.Join(tmp, "symlink")
			err = os.Symlink(dir, link)

			return link, err
		},
	},
	{
		desc:    "symlink to mounted directory",
		isMount: true,
		prepare: func(t *testing.T) (link string, err error) {
			tmp := t.TempDir()

			dir, err := os.MkdirTemp(tmp, "dir")
			if err != nil {
				return
			}

			err = tMount(t, "tmpfs", dir, "tmpfs", 0, "")
			if err != nil {
				return
			}

			link = filepath.Join(tmp, "symlink")
			err = os.Symlink(dir, link)

			return link, err
		},
	},
	{
		desc:    "symlink to a file on a different filesystem",
		isMount: false,
		prepare: func(t *testing.T) (link string, err error) {
			tmp := t.TempDir()

			mnt, err := os.MkdirTemp(tmp, "dir")
			if err != nil {
				return
			}

			err = tMount(t, "tmpfs", mnt, "tmpfs", 0, "")
			if err != nil {
				return
			}
			file, err := os.CreateTemp(mnt, "file")
			if err != nil {
				return
			}
			file.Close()
			link = filepath.Join(tmp, "link")
			err = os.Symlink(file.Name(), link)

			return link, err
		},
	},
	{
		desc:    "path whose parent is a symlink to directory on another device",
		isMount: false,
		prepare: func(t *testing.T) (path string, err error) {
			tmp := t.TempDir()

			mnt, err := os.MkdirTemp(tmp, "dir")
			if err != nil {
				return
			}

			err = tMount(t, "tmpfs", mnt, "tmpfs", 0, "")
			if err != nil {
				return
			}
			file, err := os.CreateTemp(mnt, "file")
			if err != nil {
				return
			}
			file.Close()

			// Create link -> mnt under tmp dir.
			link := filepath.Join(tmp, "link")
			err = os.Symlink(filepath.Base(mnt), link)
			// Path to check is /<tmp>/link/file.
			path = filepath.Join(link, filepath.Base(file.Name()))

			return path, err
		},
	},
	{
		desc:    "directory bind mounted to itself",
		isMount: true,
		isBind:  true,
		prepare: func(t *testing.T) (mnt string, err error) {
			mnt = t.TempDir()
			err = tMount(t, mnt, mnt, "", unix.MS_BIND, "")
			return mnt, err
		},
	},
	{
		desc:    "directory bind-mounted to other directory",
		isMount: true,
		isBind:  true,
		prepare: func(t *testing.T) (mnt string, err error) {
			dir := t.TempDir()
			mnt = t.TempDir()
			err = tMount(t, dir, mnt, "", unix.MS_BIND, "")
			return mnt, err
		},
	},
	{
		desc: "not mounted file",
		prepare: func(t *testing.T) (path string, err error) {
			dir := t.TempDir()
			file, err := os.CreateTemp(dir, "file")
			if err != nil {
				return
			}
			return file.Name(), err
		},
	},
	{
		desc:    "regular file bind-mounted to itself",
		isMount: true,
		isBind:  true,
		prepare: func(t *testing.T) (path string, err error) {
			dir := t.TempDir()

			file, err := os.CreateTemp(dir, "file")
			if err != nil {
				return
			}
			file.Close()
			path = file.Name()

			err = tMount(t, path, path, "", unix.MS_BIND, "")

			return path, err
		},
	},
	{
		desc: "not mounted socket",
		prepare: func(t *testing.T) (path string, err error) {
			dir := t.TempDir()
			path = filepath.Join(dir, "sock")
			_, err = net.Listen("unix", path)
			return path, err
		},
	},
	{
		desc:    "socket bind-mounted to itself",
		isMount: true,
		isBind:  true,
		prepare: func(t *testing.T) (path string, err error) {
			dir := t.TempDir()
			path = filepath.Join(dir, "sock")
			_, err = net.Listen("unix", path)
			if err != nil {
				return
			}
			err = tMount(t, path, path, "", unix.MS_BIND, "")

			return path, err
		},
	},
}

func requireOpenat2(t *testing.T) {
	t.Helper()
	if err := tryOpenat2(); err != nil {
		t.Skipf("openat2: %v (old kernel? need Linux 5.6+)", err)
	}
}

func tryOpenat2() error {
	fd, err := unix.Openat2(unix.AT_FDCWD, ".", &unix.OpenHow{Flags: unix.O_RDONLY})
	if err == nil {
		_ = unix.Close(fd)
	}
	return err
}

func TestMountedBy(t *testing.T) {
	openat2Supported := tryOpenat2() == nil
	checked := false
	for _, tc := range testMounts {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			m, err := tc.prepare(t)
			if err != nil {
				t.Fatalf("prepare: %v", err)
			}

			exp := tc.isMount

			// Check the public Mounted() function as a whole.
			mounted, err := Mounted(m)
			if err == nil {
				if mounted != exp {
					t.Errorf("Mounted: expected %v, got %v", exp, mounted)
				}
			} else {
				// Got an error; is it expected?
				if !(tc.isNotExist && errors.Is(err, os.ErrNotExist)) {
					t.Errorf("Mounted: unexpected error: %v", err)
				}
				// Check false is returned in error case.
				if mounted != false {
					t.Errorf("Mounted: expected false on error, got %v", mounted)
				}
			}

			// Check individual mountedBy* implementations.

			// All mountedBy* functions should be called with normalized paths.
			m, err = normalizePath(m)
			if err != nil {
				if tc.isNotExist && errors.Is(err, os.ErrNotExist) {
					return
				}
				t.Fatalf("normalizePath: %v", err)
			}

			mounted, err = mountedByMountinfo(m)
			if err != nil {
				t.Errorf("mountedByMountinfo error: %v", err)
				// Check false is returned in error case.
				if mounted != false {
					t.Errorf("MountedByMountinfo: expected false on error, got %v", mounted)
				}
			} else if mounted != exp {
				t.Errorf("mountedByMountinfo: expected %v, got %v", exp, mounted)
			}
			checked = true

			mounted, err = mountedByStat(m)
			if err != nil {
				t.Errorf("mountedByStat error: %v", err)
				// Check false is returned in error case.
				if mounted != false {
					t.Errorf("MountedByStat: expected false on error, got %v", mounted)
				}
			} else if mounted != exp && !tc.isBind { // mountedByStat can not detect bind mounts
				t.Errorf("mountedByStat: expected %v, got %v", exp, mounted)
			}

			if !openat2Supported {
				return
			}
			mounted, err = MountedByOpenat2(m)
			if err != nil {
				t.Errorf("MountedByOpenat2 error: %v", err)
				// Check false is returned in error case.
				if mounted != false {
					t.Errorf("MountedByOpenat2: expected false on error, got %v", mounted)
				}
			} else if mounted != exp {
				t.Errorf("MountedByOpenat2: expected %v, got %v", exp, mounted)
			}
		})

	}
	if !checked {
		t.Skip("no mounts to check")
	}
}

func TestMountedByOpenat2VsMountinfo(t *testing.T) {
	requireOpenat2(t)

	mounts, err := GetMounts(nil)
	if err != nil {
		t.Fatalf("GetMounts error: %v", err)
	}

	for _, mount := range mounts {
		m := mount.Mountpoint
		if m == "/" {
			// mountedBy*() won't work for /, so skip it
			// (this special case is handled by Mounted).
			continue
		}
		mounted, err := MountedByOpenat2(m)
		if err != nil {
			if !errors.Is(err, os.ErrPermission) {
				t.Errorf("MountedByOpenat2(%q) error: %+v", m, err)
			}
		} else if !mounted {
			t.Errorf("MountedByOpenat2(%q): expected true, got false", m)
		}
	}
}
