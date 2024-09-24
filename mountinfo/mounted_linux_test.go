package mountinfo

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// tMount is a wrapper for unix.Mount which is used to prepare test cases.
// It skips the test case if mounting is not possible (i.e. user is not root),
// fails the test case if there is a mount error, and installs a cleanup
// handler to undo the mount.
func tMount(t *testing.T, src, dst, fstype string, flags uintptr) {
	t.Helper()
	if os.Getuid() != 0 {
		t.Skip("root required for mounting")
	}

	err := unix.Mount(src, dst, fstype, flags, "")
	if err != nil {
		t.Fatal(&os.PathError{Path: dst, Op: "mount", Err: err})
	}
	t.Cleanup(func() {
		if err := unix.Unmount(dst, unix.MNT_DETACH); err != nil {
			t.Errorf("cleanup: unmount %q failed: %v", dst, err)
		}
	})
}

func tCreateFile(t *testing.T, dir string) string {
	t.Helper()
	file, err := os.CreateTemp(dir, "file")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	return file.Name()
}

type testMount struct {
	desc       string
	isNotExist bool
	isMount    bool
	isBind     bool
	// prepare returns a path that needs to be checked.
	// If there is an error preparing the path, the test is either skipped or failed.
	//
	// It is responsible for cleanup (by using t.Cleanup).
	prepare func(t *testing.T) string
}

var testMounts = []testMount{
	{
		desc:       "non-existent path",
		isNotExist: true,
		prepare: func(t *testing.T) string {
			return "/non/existent/path"
		},
	},
	{
		desc: "not mounted directory",
		prepare: func(t *testing.T) string {
			return t.TempDir()
		},
	},
	{
		desc:    "tmpfs mount",
		isMount: true,
		prepare: func(t *testing.T) string {
			mnt := t.TempDir()
			tMount(t, "tmpfs", mnt, "tmpfs", 0)
			return mnt
		},
	},
	{
		desc:    "tmpfs mount ending with a slash",
		isMount: true,
		prepare: func(t *testing.T) string {
			mnt := t.TempDir() + "/"
			tMount(t, "tmpfs", mnt, "tmpfs", 0)
			return mnt
		},
	},
	{
		desc:       "broken symlink",
		isNotExist: true,
		prepare: func(t *testing.T) string {
			link := filepath.Join(t.TempDir(), "broken-symlink")
			err := os.Symlink("/some/non/existent/dest", link)
			if err != nil {
				t.Fatal(err)
			}
			return link
		},
	},
	{
		desc: "symlink to not mounted directory",
		prepare: func(t *testing.T) string {
			tmp := t.TempDir()

			t.Helper()
			dir := filepath.Join(tmp, "dir")
			err := os.Mkdir(dir, 0o700)
			if err != nil {
				t.Fatal(err)
			}

			link := filepath.Join(tmp, "symlink")
			err = os.Symlink(dir, link)
			if err != nil {
				t.Fatal(err)
			}

			return link
		},
	},
	{
		desc:    "symlink to mounted directory",
		isMount: true,
		prepare: func(t *testing.T) string {
			tmp := t.TempDir()

			t.Helper()
			dir := filepath.Join(tmp, "dir")
			err := os.Mkdir(dir, 0o700)
			if err != nil {
				t.Fatal(err)
			}

			tMount(t, "tmpfs", dir, "tmpfs", 0)

			link := filepath.Join(tmp, "symlink")
			err = os.Symlink(dir, link)
			if err != nil {
				t.Fatal(err)
			}

			return link
		},
	},
	{
		desc:    "symlink to a file on a different filesystem",
		isMount: false,
		prepare: func(t *testing.T) string {
			tmp := t.TempDir()

			t.Helper()
			mnt := filepath.Join(tmp, "dir")
			err := os.Mkdir(mnt, 0o700)
			if err != nil {
				t.Fatal(err)
			}

			tMount(t, "tmpfs", mnt, "tmpfs", 0)
			file := tCreateFile(t, mnt)
			link := filepath.Join(tmp, "link")
			err = os.Symlink(file, link)
			if err != nil {
				t.Fatal(err)
			}

			return link
		},
	},
	{
		desc:    "path whose parent is a symlink to directory on another device",
		isMount: false,
		prepare: func(t *testing.T) string {
			tmp := t.TempDir()

			t.Helper()
			mnt := filepath.Join(tmp, "dir")
			err := os.Mkdir(mnt, 0o700)
			if err != nil {
				t.Fatal(err)
			}

			tMount(t, "tmpfs", mnt, "tmpfs", 0)
			file := tCreateFile(t, mnt)
			// Create link -> mnt under tmp dir.
			link := filepath.Join(tmp, "link")
			err = os.Symlink(filepath.Base(mnt), link)
			if err != nil {
				t.Fatal(err)
			}
			// Path to check is /<tmp>/link/file.
			return filepath.Join(link, filepath.Base(file))
		},
	},
	{
		desc:    "directory bind mounted to itself",
		isMount: true,
		isBind:  true,
		prepare: func(t *testing.T) string {
			mnt := t.TempDir()
			tMount(t, mnt, mnt, "", unix.MS_BIND)
			return mnt
		},
	},
	{
		desc:    "directory bind-mounted to other directory",
		isMount: true,
		isBind:  true,
		prepare: func(t *testing.T) string {
			mnt := t.TempDir()
			tMount(t, t.TempDir(), mnt, "", unix.MS_BIND)
			return mnt
		},
	},
	{
		desc: "not mounted file",
		prepare: func(t *testing.T) string {
			return tCreateFile(t, t.TempDir())
		},
	},
	{
		desc:    "regular file bind-mounted to itself",
		isMount: true,
		isBind:  true,
		prepare: func(t *testing.T) string {
			file := tCreateFile(t, t.TempDir())
			tMount(t, file, file, "", unix.MS_BIND)
			return file
		},
	},
	{
		desc: "not mounted socket",
		prepare: func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "sock")
			_, err := net.Listen("unix", path)
			if err != nil {
				t.Helper()
				t.Fatal(err)
			}
			return path
		},
	},
	{
		desc:    "socket bind-mounted to itself",
		isMount: true,
		isBind:  true,
		prepare: func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "sock")
			_, err := net.Listen("unix", path)
			if err != nil {
				t.Helper()
				t.Fatal(err)
			}
			tMount(t, path, path, "", unix.MS_BIND)

			return path
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

func testMountedFast(t *testing.T, path string, tc *testMount, openat2Supported bool) {
	mounted, sure, err := MountedFast(path)
	if err != nil {
		// Got an error; is it expected?
		if !(tc.isNotExist && errors.Is(err, os.ErrNotExist)) {
			t.Errorf("MountedFast: unexpected error: %v", err)
		}

		// In case of an error, sure and mounted must be false.
		if sure {
			t.Error("MountedFast: expected sure to be false on error")
		}
		if mounted {
			t.Error("MountedFast: expected mounted to be false on error")
		}

		// No more checks.
		return
	}

	if openat2Supported {
		if mounted != tc.isMount {
			t.Errorf("MountedFast: expected mounted to be %v, got %v", tc.isMount, mounted)
		}

		// No more checks.
		return
	}

	if tc.isBind {
		// For bind mounts, in case openat2 is not supported,
		// sure and mounted must be false.
		if sure {
			t.Error("MountedFast: expected sure to be false for a bind mount")
		}
		if mounted {
			t.Error("MountedFast: expected mounted to be false for a bind mount")
		}
	} else {
		if mounted != tc.isMount {
			t.Errorf("MountFast: expected mounted to be %v, got %v", tc.isMount, mounted)
		}
		if tc.isMount && !sure {
			t.Error("MountFast: expected sure to be true for normal mount")
		}
		if !tc.isMount && sure {
			t.Error("MountFast: expected sure to be false for non-mount")
		}
	}
}

func TestMountedBy(t *testing.T) {
	checked := false
	openat2Supported := false

	// List of individual implementations to check.
	toCheck := []func(string) (bool, error){mountedByMountinfo, mountedByStat}
	if tryOpenat2() == nil {
		openat2Supported = true
		toCheck = append(toCheck, mountedByOpenat2)
	}

	for _, tc := range testMounts {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			m := tc.prepare(t)

			// Check the public Mounted() function as a whole.
			mounted, err := Mounted(m)
			if err == nil {
				if mounted != tc.isMount {
					t.Errorf("Mounted: expected %v, got %v", tc.isMount, mounted)
				}
			} else {
				// Got an error; is it expected?
				if !(tc.isNotExist && errors.Is(err, os.ErrNotExist)) {
					t.Errorf("Mounted: unexpected error: %v", err)
				}
				// Check false is returned in error case.
				if mounted {
					t.Error("Mounted: expected false on error")
				}
			}

			// Check the public MountedFast() function as a whole.
			testMountedFast(t, m, &tc, openat2Supported)

			// Check individual mountedBy* implementations.

			// All mountedBy* functions should be called with normalized paths.
			m, err = normalizePath(m)
			if err != nil {
				if tc.isNotExist && errors.Is(err, os.ErrNotExist) {
					return
				}
				t.Fatalf("normalizePath: %v", err)
			}

			for _, fn := range toCheck {
				// Figure out function name.
				name := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()

				mounted, err = fn(m)
				if err != nil {
					t.Errorf("%s: %v", name, err)
					// Check false is returned in error case.
					if mounted {
						t.Errorf("%s: expected false on error", name)
					}
				} else if mounted != tc.isMount {
					if tc.isBind && strings.HasSuffix(name, "mountedByStat") {
						// mountedByStat can not detect bind mounts.
					} else {
						t.Errorf("%s: expected %v, got %v", name, tc.isMount, mounted)
					}
				}
				checked = true
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
		mounted, err := mountedByOpenat2(m)
		if err != nil {
			if !errors.Is(err, os.ErrPermission) {
				t.Errorf("mountedByOpenat2(%q) error: %+v", m, err)
			}
		} else if !mounted {
			t.Errorf("mountedByOpenat2(%q): expected true, got false", m)
		}
	}
}

// TestMountedRoot checks that Mounted* functions always return true for root
// directory (since / is always mounted).
func TestMountedRoot(t *testing.T) {
	for _, path := range []string{
		"/",
		"/../../",
		"/tmp/..",
		strings.Repeat("../", unix.PathMax/3), // Hope $CWD is not too deep down.
	} {
		mounted, err := Mounted(path)
		if err != nil || !mounted {
			t.Errorf("Mounted(%q): expected true, <nil>; got %v, %v", path, mounted, err)
		}

		mounted, sure, err := MountedFast(path)
		if err != nil || !mounted || !sure {
			t.Errorf("MountedFast(%q): expected true, true, <nil>; got %v, %v, %v", path, mounted, sure, err)
		}
	}
}
