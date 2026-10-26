package mountinfo

import "testing"

func TestPrefixFilter(t *testing.T) {
	tests := []struct {
		prefix     string
		mountPoint string
		shouldSkip bool
	}{
		{prefix: "/a", mountPoint: "/a", shouldSkip: false},
		{prefix: "/a", mountPoint: "/a/b", shouldSkip: false},
		{prefix: "/a", mountPoint: "/aa", shouldSkip: true},
		{prefix: "/a", mountPoint: "/aa/b", shouldSkip: true},
		{prefix: "/", mountPoint: "/", shouldSkip: false},
		{prefix: "/", mountPoint: "/a", shouldSkip: false},
		{prefix: "/", mountPoint: "/a/b", shouldSkip: false},

		// invalid prefix: prefix path must be cleaned and have no trailing slash
		{prefix: "/a/", mountPoint: "/a", shouldSkip: true},
		{prefix: "/a/", mountPoint: "/a/b", shouldSkip: true},
	}
	for _, tc := range tests {
		filter := PrefixFilter(tc.prefix)
		skip, _ := filter(&Info{Mountpoint: tc.mountPoint})
		if skip != tc.shouldSkip {
			if tc.shouldSkip {
				t.Errorf("prefix %q: expected %q to be skipped", tc.prefix, tc.mountPoint)
			} else {
				t.Errorf("prefix %q: expected %q not to be skipped", tc.prefix, tc.mountPoint)
			}
		}
	}
}

func TestParentsFilter(t *testing.T) {
	tests := []struct {
		path       string
		mountPoint string
		shouldSkip bool
	}{
		{path: "/a/b", mountPoint: "/", shouldSkip: false},
		{path: "/a/b", mountPoint: "/a", shouldSkip: false},
		{path: "/a/b", mountPoint: "/a/b", shouldSkip: false},
		{path: "/a/b", mountPoint: "/a/b/c", shouldSkip: true},
		{path: "/a/b", mountPoint: "/b", shouldSkip: true},
		{path: "/aa", mountPoint: "/a", shouldSkip: true},
		{path: "/a/bb", mountPoint: "/a/b", shouldSkip: true},
		{path: "/", mountPoint: "/", shouldSkip: false},
		{path: "/", mountPoint: "/a", shouldSkip: true},
	}
	for _, tc := range tests {
		filter := ParentsFilter(tc.path)
		skip, _ := filter(&Info{Mountpoint: tc.mountPoint})
		if skip != tc.shouldSkip {
			if tc.shouldSkip {
				t.Errorf("path %q: expected %q to be skipped", tc.path, tc.mountPoint)
			} else {
				t.Errorf("path %q: expected %q not to be skipped", tc.path, tc.mountPoint)
			}
		}
	}
}
