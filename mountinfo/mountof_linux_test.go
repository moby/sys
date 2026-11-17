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
