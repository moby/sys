package mountinfo

import (
	"errors"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

const (
	notMounted = "not-mounted"
)

func prepareMounts(t *testing.T) (dir string, mounts []string, err error) {
	dir, err = ioutil.TempDir("", t.Name())
	if err != nil {
		return
	}

	// A real (tmpfs) mount.
	mnt := filepath.Join(dir, "tmpfs-mount")
	err = os.Mkdir(mnt, 0750)
	if err != nil {
		return
	}

	err = unix.Mount("tmpfs", mnt, "tmpfs", 0, "")
	if err != nil {
		err = &os.PathError{Op: "mount", Path: mnt, Err: err}
		return
	}
	mounts = append(mounts, mnt)

	// A directory bind-mounted to itself.
	mnt = filepath.Join(dir, "bind-mount-dir")
	err = os.Mkdir(mnt, 0750)
	if err != nil {
		return
	}

	err = unix.Mount(mnt, mnt, "", unix.MS_BIND, "")
	if err != nil {
		err = &os.PathError{Op: "mount", Path: mnt, Err: err}
		return
	}
	mounts = append(mounts, mnt)

	// A directory bind-mounted to other directory.
	src := filepath.Join(dir, "some-dir")
	err = os.Mkdir(src, 0750)
	if err != nil {
		return
	}

	mnt = filepath.Join(dir, "bind-mounted-dir2")
	err = os.Mkdir(mnt, 0750)
	if err != nil {
		return
	}
	err = unix.Mount(src, mnt, "", unix.MS_BIND, "")
	if err != nil {
		err = &os.PathError{Op: "mount", Path: mnt, Err: err}
		return
	}
	mounts = append(mounts, mnt)

	// A regular file bind-mounted to itself.
	mnt = filepath.Join(dir, "bind-mount-file")
	err = ioutil.WriteFile(mnt, []byte(""), 0640)
	if err != nil {
		return
	}

	err = unix.Mount(mnt, mnt, "", unix.MS_BIND, "")
	if err != nil {
		err = &os.PathError{Op: "mount", Path: mnt, Err: err}
		return
	}
	mounts = append(mounts, mnt)

	// Not mounted socket.
	sock := filepath.Join(dir, notMounted+".sock")
	_, err = net.Listen("unix", sock)
	if err != nil {
		return
	}
	mounts = append(mounts, sock)

	// Bind-mounted socket.
	mnt = filepath.Join(dir, "bind-mounted-socket")
	err = ioutil.WriteFile(mnt, []byte(""), 0640)
	if err != nil {
		return
	}
	err = unix.Mount(sock, mnt, "", unix.MS_BIND, "")
	if err != nil {
		err = &os.PathError{Op: "mount", Path: mnt, Err: err}
		return
	}
	mounts = append(mounts, mnt)

	// Not mounted directory.
	mnt = filepath.Join(dir, notMounted+"-dir")
	err = os.Mkdir(mnt, 0750)
	if err != nil {
		return
	}
	mounts = append(mounts, mnt)

	// Not mounted file.
	mnt = filepath.Join(dir, notMounted+"-file")
	err = ioutil.WriteFile(mnt, []byte(""), 0640)
	if err != nil {
		return
	}
	mounts = append(mounts, mnt)

	// A broken not-mounted symlink.
	symlink := filepath.Join(dir, notMounted+"-broken-symlink")
	err = unix.Symlink("non-existent-dest", symlink)
	if err != nil {
		err = &os.PathError{Op: "symlink", Path: symlink, Err: err}
		return
	}
	mounts = append(mounts, symlink)

	// A valid not-mounted symlink.
	dst := filepath.Join(dir, "file")
	err = ioutil.WriteFile(dst, []byte(""), 0640)
	if err != nil {
		return
	}
	symlink = filepath.Join(dir, notMounted+"-valid-symlink")
	err = unix.Symlink(dst, symlink)
	if err != nil {
		err = &os.PathError{Op: "symlink", Path: symlink, Err: err}
		return
	}
	mounts = append(mounts, symlink)

	// A valid bind-mounted symlink
	mnt = filepath.Join(dir, "bind-mounted-symlink")
	err = ioutil.WriteFile(mnt, []byte(""), 0640)
	if err != nil {
		return
	}
	err = unix.Mount(symlink, mnt, "", unix.MS_BIND, "")
	if err != nil {
		err = &os.PathError{Op: "mount", Path: mnt, Err: err}
		return
	}
	mounts = append(mounts, mnt)

	return
}

func cleanupMounts(t *testing.T, dir string, mounts []string) {
	for _, m := range mounts {
		if strings.Contains(m, notMounted) {
			continue
		}
		err := unix.Unmount(m, unix.MNT_DETACH)
		if err != nil {
			t.Logf("can't umount %s: %v", m, err)
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Log(err)
	}
}

func TestMountedBy(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}

	dir, mounts, err := prepareMounts(t)
	defer cleanupMounts(t, dir, mounts)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	checked := false
	for _, m := range mounts {
		exp := !strings.Contains(m, notMounted)

		mounted, err := mountedByMountinfo(m)
		if err != nil {
			t.Errorf("mountedByMountinfo(%q) error: %v", m, err)
		} else if mounted != exp {
			t.Errorf("mountedByMountinfo(%q): expected %v, got %v", m, exp, mounted)
		}

		checked = true
		mounted, err = mountedByOpenat2(m)
		if err != nil {
			t.Errorf("mountedByOpenat2(%q) error: %v", m, err)
		} else if mounted != exp {
			t.Errorf("mountedByOpenat2(%q): expected %v, got %v", m, exp, mounted)
		}

		mounted, err = mountedByStat(m)
		// mountedByStat can't detect bind mounts, returning
		// errNotSure in case it can't reliably detect the mount.
		if strings.Contains(m, "bind") {
			exp = false
		}
		if err != nil {
			t.Errorf("mountedByStat(%q) error: %v", m, err)
		} else {
			if mounted != exp {
				t.Errorf("mountedByStat(%q): expected %v, got %v", m, exp, mounted)
			}
		}
	}
	if !checked {
		t.Skip("no mounts to check can be found")
	}
}

func TestMountedByOpenat2VsMountinfo(t *testing.T) {
	fd, err := unix.Openat2(unix.AT_FDCWD, ".", &unix.OpenHow{Flags: unix.O_RDONLY})
	if err != nil {
		t.Skipf("openat2: %v (old kernel? need Linux 5.6+)", err)
	}
	unix.Close(fd)

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
