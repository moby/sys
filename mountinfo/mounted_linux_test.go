package mountinfo

import (
	"errors"
	"net"
	"os"
	"os/exec"
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

func requireStatx(t *testing.T) {
	t.Helper()
	if err := tryStatx(); err != nil {
		t.Skipf("statx: %v (old kernel? need Linux 5.8+)", err)
	}
}

func tryStatx() error {
	_, err := mountedByStatx("/")
	return err
}

func tryOpenat2() error {
	fd, err := unix.Openat2(unix.AT_FDCWD, ".", &unix.OpenHow{Flags: unix.O_RDONLY})
	if err == nil {
		_ = unix.Close(fd)
	}
	return err
}

func testMountedFast(t *testing.T, path string, tc *testMount, fastSupported bool) {
	mounted, sure, err := MountedFast(path)
	if err != nil {
		// Got an error; is it expected?
		if !tc.isNotExist || !errors.Is(err, os.ErrNotExist) {
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

	if fastSupported {
		if mounted != tc.isMount {
			t.Errorf("MountedFast: expected mounted to be %v, got %v", tc.isMount, mounted)
		}
		if !sure {
			t.Error("MountedFast: expected sure to be true")
		}

		// No more checks.
		return
	}

	if tc.isBind {
		// For bind mounts, in case neither statx nor openat2 is supported,
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
	// Whether MountedFast can reliably detect bind mounts.
	fastSupported := false

	// List of individual implementations to check.
	toCheck := []func(string) (bool, error){mountedByMountinfo, mountedByStat}
	if tryOpenat2() == nil {
		fastSupported = true
		toCheck = append(toCheck, mountedByOpenat2)
	}
	if tryStatx() == nil {
		fastSupported = true
		toCheck = append(toCheck, mountedByStatx)
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
				if !tc.isNotExist || !errors.Is(err, os.ErrNotExist) {
					t.Errorf("Mounted: unexpected error: %v", err)
				}
				// Check false is returned in error case.
				if mounted {
					t.Error("Mounted: expected false on error")
				}
			}

			// Check the public MountedFast() function as a whole.
			testMountedFast(t, m, &tc, fastSupported)

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
	testMountedByVsMountinfo(t, "mountedByOpenat2", mountedByOpenat2)
}

func TestMountedByStatxVsMountinfo(t *testing.T) {
	requireStatx(t)
	testMountedByVsMountinfo(t, "mountedByStatx", mountedByStatx)
}

func testMountedByVsMountinfo(t *testing.T, name string, fn func(string) (bool, error)) {
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
		if !mountReachable(t, mount) {
			continue
		}
		mounted, err := fn(m)
		if err != nil {
			if !errors.Is(err, os.ErrPermission) {
				t.Errorf("%s(%q) error: %+v", name, m, err)
			}
		} else if !mounted {
			t.Errorf("%s(%q): expected true, got false", name, m)
		}
	}
}

// mountReachable checks whether the mount can be accessed via its mount point,
// as a mount may be hidden by a later overmount of its mount point (or one of
// its parents), while its mountinfo entry remains.
func mountReachable(t *testing.T, mount *Info) bool {
	t.Helper()
	var st unix.Statx_t
	err := unix.Statx(unix.AT_FDCWD, mount.Mountpoint, unix.AT_NO_AUTOMOUNT, unix.STATX_MNT_ID, &st)
	if err != nil {
		t.Logf("skipping %q: statx: %v", mount.Mountpoint, err)
		return false
	}
	if st.Mask&unix.STATX_MNT_ID == 0 {
		return true // Can't tell, assume it is.
	}
	if st.Mnt_id != uint64(mount.ID) {
		t.Logf("skipping %q: mount ID %d, want %d (overmounted?)", mount.Mountpoint, st.Mnt_id, mount.ID)
		return false
	}
	return true
}

// TestMountedRoot checks that Mounted* functions always return true for root
// directory (since / is always mounted).
func TestMountedRoot(t *testing.T) {
	for _, path := range []string{
		"/",
		"/../../",
		"/tmp/..",
		strings.Repeat("../", unix.PathMax/3), // Hope $CWD is not too deep down.
		"/proc/self/root",
		rootLink(t),
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

// rootLink creates a symlink to / and returns its path.
func rootLink(t *testing.T) string {
	t.Helper()
	link := filepath.Join(t.TempDir(), "root")
	if err := os.Symlink("/", link); err != nil {
		t.Fatal(err)
	}
	return link
}

// TestMountedRootChroot checks that Mounted* functions return true for
// a path resolving to root in a chroot, where root is not a mount root.
func TestMountedRootChroot(t *testing.T) {
	const env = "MOUNTINFO_TEST_CHROOT"
	if dir := os.Getenv(env); dir != "" {
		// In the child: chroot and check.
		if err := unix.Chroot(dir); err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir("/"); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/", "/root", "/dir/.."} {
			mounted, err := Mounted(path)
			if err != nil || !mounted {
				t.Errorf("Mounted(%q): expected true, <nil>; got %v, %v", path, mounted, err)
			}
			mounted, sure, err := MountedFast(path)
			if err != nil || !mounted || !sure {
				t.Errorf("MountedFast(%q): expected true, true, <nil>; got %v, %v, %v", path, mounted, sure, err)
			}
		}
		return
	}

	if os.Getuid() != 0 {
		t.Skip("root required")
	}
	dir := t.TempDir()
	if err := os.Symlink("/", filepath.Join(dir, "root")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if m, err := Mounted(dir); err != nil || m {
		t.Skipf("%s is a mount point (or error: %v)", dir, err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMountedRootChroot$", "-test.v")
	cmd.Env = append(os.Environ(), env+"="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child failed: %v\n%s", err, out)
	}
}
