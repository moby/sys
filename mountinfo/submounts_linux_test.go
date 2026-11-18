package mountinfo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// submountsImpls returns all implementations of submounts to check.
func submountsImpls() map[string]func(string) ([]*Info, error) {
	return map[string]func(string) ([]*Info, error){
		"Submounts": Submounts,
		"SubmountsNoStatmount": func(path string) ([]*Info, error) {
			noStatmount = true
			defer func() { noStatmount = false }()
			return Submounts(path)
		},
		"byPrefix": submountsByPrefixSlow,
	}
}

// checkSubmounts checks that all submounts implementations return the same
// result for path, and returns it. If isNotExist is set, all implementations
// are expected to return ErrNotExist.
func checkSubmounts(t *testing.T, path string, isNotExist bool) []Info {
	t.Helper()
	var want []Info
	first := true
	for name, fn := range submountsImpls() {
		mounts, err := fn(path)
		if isNotExist {
			if !errors.Is(err, os.ErrNotExist) {
				t.Errorf("%s(%q): want ErrNotExist, got %v", name, path, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s(%q): %v", name, path, err)
			continue
		}
		got := make([]Info, len(mounts))
		for i, m := range mounts {
			got[i] = infoNoUnique(m)
		}
		if first {
			want, first = got, false
			continue
		}
		if len(got) != len(want) {
			t.Errorf("%s(%q): got %d mounts, want %d:\n got  %+v\n want %+v", name, path, len(got), len(want), got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s(%q): mismatch at %d:\n got  %+v\n want %+v", name, path, i, got[i], want[i])
			}
		}
	}
	return want
}

func TestSubmounts(t *testing.T) {
	base := t.TempDir()
	tMount(t, "tmpfs", base, "tmpfs", 0)
	mkdir := func(p string) string {
		t.Helper()
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		return p
	}

	a := mkdir(filepath.Join(base, "a"))
	tMount(t, "tmpfs", a, "tmpfs", 0)
	b := mkdir(filepath.Join(a, "b"))
	tMount(t, "tmpfs", b, "tmpfs", 0)
	// Stack another mount on a, hiding a/b.
	tMount(t, "tmpfs", a, "tmpfs", 0)
	// Directory bind mount.
	c := mkdir(filepath.Join(base, "c"))
	tMount(t, t.TempDir(), c, "", unix.MS_BIND)
	// File bind mount.
	f := tCreateFile(t, base)
	tMount(t, f, f, "", unix.MS_BIND)
	// A directory which is not a mount point, with a mount under it.
	d := mkdir(filepath.Join(base, "d"))
	e := mkdir(filepath.Join(d, "e"))
	tMount(t, "tmpfs", e, "tmpfs", 0)
	// Symlink to a.
	link := filepath.Join(base, "link")
	if err := os.Symlink(a, link); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		path       string
		isNotExist bool
		want       []string // mount points, in order
	}{
		{path: base, want: []string{base, a, b, a, c, f, e}},
		{path: base + "/", want: []string{base, a, b, a, c, f, e}},
		{path: a, want: []string{a, b, a}},
		{path: link, want: []string{a, b, a}},
		{path: c, want: []string{c}},
		{path: f, want: []string{f}},
		{path: d, want: []string{e}},
		{path: e, want: []string{e}},
		{path: filepath.Join(base, "nonexistent"), isNotExist: true},
		{path: filepath.Join(link, "nonexistent"), isNotExist: true},
	} {
		got := checkSubmounts(t, tc.path, tc.isNotExist)
		if tc.isNotExist {
			continue
		}
		var gotMP []string
		for _, m := range got {
			gotMP = append(gotMP, m.Mountpoint)
		}
		if len(gotMP) != len(tc.want) {
			t.Errorf("%s: got %q, want %q", tc.path, gotMP, tc.want)
			continue
		}
		for i := range gotMP {
			if gotMP[i] != tc.want[i] {
				t.Errorf("%s: got %q, want %q", tc.path, gotMP, tc.want)
				break
			}
		}
	}
}

// TestSubmountsAllMounts checks that all submounts implementations agree
// for all mount points in the system.
func TestSubmountsAllMounts(t *testing.T) {
	mounts, err := GetMounts(nil)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	seen := map[string]bool{}
	for _, m := range mounts {
		mp := m.Mountpoint
		if seen[mp] || mp == "/" {
			continue
		}
		seen[mp] = true
		info, err := MountOf(mp)
		if err != nil || info.Mountpoint != mp {
			// Inaccessible, or shadowed by a mount on top of its parent.
			continue
		}
		checkSubmounts(t, mp, false)
		checked++
	}
	t.Logf("checked %d mount points", checked)
}

func BenchmarkSubmounts(b *testing.B) {
	path, _ := os.Getwd()
	for name, fn := range submountsImpls() {
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				if _, err := fn(path); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
