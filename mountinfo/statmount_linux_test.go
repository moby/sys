package mountinfo

import (
	"errors"
	"testing"
)

func requireStatmount(t testing.TB) {
	t.Helper()
	if _, err := getMountsStatmount(func(*Info) (bool, bool) { return false, true }); err != nil {
		if errors.Is(err, errStatmountUnsupported) {
			t.Skip("statmount not supported (need Linux 6.13+)")
		}
		t.Fatal(err)
	}
}

// infoNoUnique returns a copy of i with unique IDs cleared,
// so it can be compared with Info parsed from mountinfo.
func infoNoUnique(i *Info) Info {
	c := *i
	c.UniqueID, c.ParentUniqueID = 0, 0
	return c
}

// TestStatmountVsMountinfo checks that getMountsStatmount returns the same
// information as parsing mountinfo.
func TestStatmountVsMountinfo(t *testing.T) {
	requireStatmount(t)

	// Mounts can change between the two calls; retry a few times.
	var diff []string
	for range 3 {
		diff = nil
		mi, err := GetMounts(nil)
		if err != nil {
			t.Fatal(err)
		}
		sm, err := getMountsStatmount(nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("mountinfo: %d entries, statmount: %d entries", len(mi), len(sm))
		if len(mi) != len(sm) {
			diff = append(diff, "number of entries differ")
		}
		byID := make(map[int]*Info, len(sm))
		for _, s := range sm {
			if s.UniqueID == 0 || s.ParentUniqueID == 0 {
				t.Errorf("statmount: unique ID not set: %+v", s)
			}
			byID[s.ID] = s
		}
		for _, m := range mi {
			s, ok := byID[m.ID]
			if !ok {
				diff = append(diff, "missing in statmount: "+m.Mountpoint)
				continue
			}
			if got, want := infoNoUnique(s), *m; got != want {
				t.Errorf("mismatch:\n statmount: %+v\n mountinfo: %+v", got, want)
			}
		}
		if diff == nil {
			break
		}
	}
	for _, d := range diff {
		t.Error(d)
	}
}

func BenchmarkGetMountsMountinfo(b *testing.B) {
	for b.Loop() {
		if _, err := GetMounts(nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetMountsStatmount(b *testing.B) {
	requireStatmount(b)
	for b.Loop() {
		if _, err := getMountsStatmount(nil); err != nil {
			b.Fatal(err)
		}
	}
}
