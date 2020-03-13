// +build !windows

package mountinfo

import "testing"

func TestGetMounts(t *testing.T) {
	mounts, err := GetMounts(nil)
	if err != nil {
		t.Fatal(err)
	}

	root := false
	for _, entry := range mounts {
		if entry.Mountpoint == "/" {
			root = true
		}
	}

	if !root {
		t.Fatal("/ should be mounted at least")
	}
}
