package mountinfo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// mountOfImpls returns all implementations of mountOf to check.
// Each one takes a non-normalized path.
func mountOfImpls() map[string]func(string) (*Info, error) {
	return map[string]func(string) (*Info, error){
		"MountOf": MountOf,
		"byID": func(path string) (*Info, error) {
			var st unix.Statx_t
			if err := unix.Statx(unix.AT_FDCWD, path, unix.AT_NO_AUTOMOUNT, unix.STATX_MNT_ID, &st); err != nil {
				return nil, &os.PathError{Op: "statx", Path: path, Err: err}
			}
			return mountOfByID(int(st.Mnt_id))
		},
		"byParents": mountOfByParentsSlow,
		"MountOfNoStatmount": func(path string) (*Info, error) {
			noStatmount = true
			defer func() { noStatmount = false }()
			return MountOf(path)
		},
	}
}

func TestMountOf(t *testing.T) {
	statmountOK := true
	if _, err := getMountsStatmount(func(*Info) (bool, bool) { return false, true }); err != nil {
		statmountOK = false
	}

	for _, tc := range testMounts {
		t.Run(tc.desc, func(t *testing.T) {
			path := tc.prepare(t)

			var want *Info
			for name, fn := range mountOfImpls() {
				info, err := fn(path)
				if tc.isNotExist {
					if !errors.Is(err, os.ErrNotExist) {
						t.Errorf("%s: want ErrNotExist, got %v", name, err)
					}
					continue
				}
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if name == "MountOf" && statmountOK && info.UniqueID == 0 {
					t.Errorf("%s: UniqueID not set", name)
				}
				got := infoNoUnique(info)
				if want == nil {
					want = &got
				} else if got != *want {
					t.Errorf("%s: mismatch:\n got  %+v\n want %+v", name, got, *want)
				}
			}
			if tc.isNotExist {
				return
			}
			norm, err := normalizePath(path)
			if err != nil {
				t.Fatal(err)
			}
			if isMount := want.Mountpoint == norm; isMount != tc.isMount {
				t.Errorf("expected isMount %v, got Mountpoint %q for %q", tc.isMount, want.Mountpoint, norm)
			}
		})
	}
}

// TestMountOfOvermount checks that MountOf returns the topmost mount if
// several mounts are stacked on the same mount point.
func TestMountOfOvermount(t *testing.T) {
	dir := t.TempDir()
	tMount(t, "tmpfs", dir, "tmpfs", 0)
	tMount(t, "tmpfs", dir, "tmpfs", unix.MS_RDONLY)
	file := filepath.Join(dir, "nonexistent")

	for name, fn := range mountOfImpls() {
		info, err := fn(dir)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if info.Mountpoint != dir || info.Options[:2] != "ro" {
			t.Errorf("%s: expected topmost (ro) mount on %s, got %+v", name, dir, info)
		}
		if _, err := fn(file); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s(%s): want ErrNotExist, got %v", name, file, err)
		}
	}
}

// TestMountOfAllMounts checks that all MountOf implementations agree
// for all mount points in the system.
func TestMountOfAllMounts(t *testing.T) {
	mounts, err := GetMounts(nil)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	seen := map[string]bool{}
	for _, m := range mounts {
		mp := m.Mountpoint
		if seen[mp] {
			continue
		}
		seen[mp] = true
		info, err := MountOf(mp)
		if err != nil {
			if errors.Is(err, os.ErrPermission) || errors.Is(err, os.ErrNotExist) {
				continue
			}
			t.Errorf("MountOf(%q): %v", mp, err)
			continue
		}
		if info.Mountpoint != mp {
			// Mount point is shadowed by a mount on top of its parent.
			t.Logf("MountOf(%q): shadowed by %q", mp, info.Mountpoint)
			continue
		}
		for name, fn := range mountOfImpls() {
			other, err := fn(mp)
			if err != nil {
				t.Errorf("%s(%q): %v", name, mp, err)
				continue
			}
			if got, want := infoNoUnique(other), infoNoUnique(info); got != want {
				t.Errorf("%s(%q): mismatch:\n got  %+v\n want %+v", name, mp, got, want)
			}
		}
		checked++
	}
	t.Logf("checked %d mount points", checked)
}

func BenchmarkMountOf(b *testing.B) {
	path, _ := os.Getwd()
	for name, fn := range mountOfImpls() {
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				if _, err := fn(path); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestMountOfPropagation(t *testing.T) {
	makeMount := func(t *testing.T, src, fstype string, flags uintptr) string {
		t.Helper()
		dir := t.TempDir()
		tMount(t, src, dir, fstype, flags)
		return dir
	}
	setPropagation := func(t *testing.T, dir string, flags uintptr) {
		t.Helper()
		if err := unix.Mount("", dir, "", flags, ""); err != nil {
			t.Fatal(&os.PathError{Op: "mount", Path: dir, Err: err})
		}
	}

	shared := makeMount(t, "tmpfs", "tmpfs", 0)
	setPropagation(t, shared, unix.MS_SHARED)
	sharedInfo, err := MountOf(shared)
	if err != nil {
		t.Fatal(err)
	}
	group, ok := sharedInfo.PeerGroup()
	if !ok {
		t.Fatalf("expected %s to be shared, got %+v", shared, sharedInfo)
	}

	slave := makeMount(t, shared, "", unix.MS_BIND)
	setPropagation(t, slave, unix.MS_SLAVE)

	slaveShared := makeMount(t, shared, "", unix.MS_BIND)
	setPropagation(t, slaveShared, unix.MS_SLAVE)
	setPropagation(t, slaveShared, unix.MS_SHARED)

	unbindable := makeMount(t, "tmpfs", "tmpfs", 0)
	setPropagation(t, unbindable, unix.MS_UNBINDABLE)

	private := makeMount(t, "tmpfs", "tmpfs", 0)
	setPropagation(t, private, unix.MS_PRIVATE)

	for _, tc := range []struct {
		desc       string
		path       string
		shared     bool
		master     int // 0 means not a slave
		unbindable bool
	}{
		{desc: "shared", path: shared, shared: true},
		{desc: "slave", path: slave, master: group},
		{desc: "slave and shared", path: slaveShared, shared: true, master: group},
		{desc: "unbindable", path: unbindable, unbindable: true},
		{desc: "private", path: private},
	} {
		for name, fn := range mountOfImpls() {
			info, err := fn(tc.path)
			if err != nil {
				t.Fatalf("%s: %s: %v", tc.desc, name, err)
			}
			if _, ok := info.PeerGroup(); ok != tc.shared {
				t.Errorf("%s: %s: PeerGroup: got %v, want %v (%q)", tc.desc, name, ok, tc.shared, info.Optional)
			}
			master, _ := info.Master()
			if master != tc.master {
				t.Errorf("%s: %s: Master: got %d, want %d (%q)", tc.desc, name, master, tc.master, info.Optional)
			}
			if _, ok := info.PropagateFrom(); ok {
				t.Errorf("%s: %s: PropagateFrom: unexpected (%q)", tc.desc, name, info.Optional)
			}
			if got := info.Unbindable(); got != tc.unbindable {
				t.Errorf("%s: %s: Unbindable: got %v, want %v (%q)", tc.desc, name, got, tc.unbindable, info.Optional)
			}
		}
	}
}
