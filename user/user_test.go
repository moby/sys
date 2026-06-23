package user

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestParseLine(t *testing.T) {
	var (
		a, b string
		c    []string
		d    int
	)

	parseLine([]byte(""), &a, &b)
	if a != "" || b != "" {
		t.Fatalf("a and b should be empty ('%v', '%v')", a, b)
	}

	parseLine([]byte("a"), &a, &b)
	if a != "a" || b != "" {
		t.Fatalf("a should be 'a' and b should be empty ('%v', '%v')", a, b)
	}

	parseLine([]byte("bad boys:corny cows"), &a, &b)
	if a != "bad boys" || b != "corny cows" {
		t.Fatalf("a should be 'bad boys' and b should be 'corny cows' ('%v', '%v')", a, b)
	}

	parseLine([]byte(""), &c)
	if len(c) != 0 {
		t.Fatalf("c should be empty (%#v)", c)
	}

	parseLine([]byte("d,e,f:g:h:i,j,k"), &c, &a, &b, &c)
	if a != "g" || b != "h" || len(c) != 3 || c[0] != "i" || c[1] != "j" || c[2] != "k" {
		t.Fatalf("a should be 'g', b should be 'h', and c should be ['i','j','k'] ('%v', '%v', '%#v')", a, b, c)
	}

	parseLine([]byte("::::::::::"), &a, &b, &c)
	if a != "" || b != "" || len(c) != 0 {
		t.Fatalf("a, b, and c should all be empty ('%v', '%v', '%#v')", a, b, c)
	}

	parseLine([]byte("not a number"), &d)
	if d != 0 {
		t.Fatalf("d should be 0 (%v)", d)
	}

	parseLine([]byte("b:12:c"), &a, &d, &b)
	if a != "b" || b != "c" || d != 12 {
		t.Fatalf("a should be 'b' and b should be 'c', and d should be 12 ('%v', '%v', %v)", a, b, d)
	}
}

func TestParsePasswdFilter(t *testing.T) {
	users, err := ParsePasswdFilter(strings.NewReader(`
root:x:0:0:root:/root:/bin/bash
adm:x:3:4:adm:/var/adm:/bin/false
this is just some garbage data
`), nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("Expected 3 users, got %v", len(users))
	}
	if users[0].Uid != 0 || users[0].Name != "root" {
		t.Fatalf("Expected users[0] to be 0 - root, got %v - %v", users[0].Uid, users[0].Name)
	}
	if users[1].Uid != 3 || users[1].Name != "adm" {
		t.Fatalf("Expected users[1] to be 3 - adm, got %v - %v", users[1].Uid, users[1].Name)
	}
}

func TestParseGroupFilter(t *testing.T) {
	groups, err := ParseGroupFilter(strings.NewReader(`
root:x:0:root
adm:x:4:root,adm,daemon
this is just some garbage data
`+largeGroup()), nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(groups) != 4 {
		t.Fatalf("Expected 4 groups, got %v", len(groups))
	}
	if groups[0].Gid != 0 || groups[0].Name != "root" || len(groups[0].List) != 1 {
		t.Fatalf("Expected groups[0] to be 0 - root - 1 member, got %v - %v - %v", groups[0].Gid, groups[0].Name, len(groups[0].List))
	}
	if groups[1].Gid != 4 || groups[1].Name != "adm" || len(groups[1].List) != 3 {
		t.Fatalf("Expected groups[1] to be 4 - adm - 3 members, got %v - %v - %v", groups[1].Gid, groups[1].Name, len(groups[1].List))
	}
}

// TestParseGroupFileCapsReads asserts the boundary behavior of the read cap:
// well below, ending exactly at, and past maxUserFileBytes.
func TestParseGroupFileCapsReads(t *testing.T) {
	beyond := []byte("\nbeyond:x:42:\n")

	for _, tc := range []struct {
		name     string
		padBytes int
		expGIDs  []int
		expErr   string
	}{
		{
			name:     "pad below cap, beyond is parsed",
			padBytes: 100,
			expGIDs:  []int{42},
		},
		{
			name:     "beyond ends exactly at cap, is parsed",
			padBytes: maxUserFileBytes - len(beyond),
			expGIDs:  []int{42},
		},
		{
			name:     "pad past cap, read errors out",
			padBytes: maxUserFileBytes,
			expErr:   "file exceeds",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			fileName := filepath.Join(tmpDir, "etc-group")

			data := append(bytes.Repeat([]byte{0}, tc.padBytes), beyond...)
			err := os.WriteFile(fileName, data, 0o644)
			if err != nil {
				t.Fatal(err)
			}
			gids, err := ParseGroupFile(fileName)
			if tc.expErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tc.expErr) {
					t.Fatalf("unexpected error: %s", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			haveGids := make([]int, 0, len(gids))
			for _, g := range gids {
				haveGids = append(haveGids, g.Gid)
			}
			if !slices.Equal(haveGids, tc.expGIDs) {
				t.Fatalf("unexpected gids: got %v, want %v", gids, tc.expGIDs)
			}
		})
	}
}

// TestTestParseGroupFileCapsReadsonRegularFile verifies that non-regular files
// are refused.
func TestTestParseGroupFileCapsReadsonRegularFile(t *testing.T) {
	fileName := t.TempDir()
	_, err := ParseGroupFile(fileName)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("unexpected error: %s", err)
	}
}

func TestGetExecUser(t *testing.T) {
	const passwdContent = `
root:x:0:0:root user:/root:/bin/bash
adm:x:42:43:adm:/var/adm:/bin/false
111:x:222:333::/var/garbage
odd:x:111:112::/home/odd:::::
2147483647:x:0:0:maxint32:/root:/bin/bash
2147483648:x:0:0:toolarge:/root:/bin/bash
9223372036854775807:x:0:0:maxint64:/root:/bin/bash
user7456:x:7456:100:Vasya:/home/user7456
this is just some garbage data
`
	groupContent := `
root:x:0:root
adm:x:43:
grp:x:1234:root,adm,user7456
444:x:555:111
odd:x:444:
2147483647:x:1235:
2147483648:x:1236:
9223372036854775807:x:1237:
this is just some garbage data
` + largeGroup()

	defaultExecUser := ExecUser{
		Uid:   8888,
		Gid:   8888,
		Sgids: []int{8888},
		Home:  "/8888",
	}

	tests := []struct {
		ref      string
		expected ExecUser
	}{
		{
			ref: "root",
			expected: ExecUser{
				Uid:   0,
				Gid:   0,
				Sgids: []int{0, 1234},
				Home:  "/root",
			},
		},
		{
			ref: "adm",
			expected: ExecUser{
				Uid:   42,
				Gid:   43,
				Sgids: []int{1234},
				Home:  "/var/adm",
			},
		},
		{
			ref: "root:adm",
			expected: ExecUser{
				Uid:   0,
				Gid:   43,
				Sgids: defaultExecUser.Sgids,
				Home:  "/root",
			},
		},
		{
			ref: "adm:1234",
			expected: ExecUser{
				Uid:   42,
				Gid:   1234,
				Sgids: defaultExecUser.Sgids,
				Home:  "/var/adm",
			},
		},
		{
			ref: "42:1234",
			expected: ExecUser{
				Uid:   42,
				Gid:   1234,
				Sgids: defaultExecUser.Sgids,
				Home:  "/var/adm",
			},
		},
		{
			ref: "1337:1234",
			expected: ExecUser{
				Uid:   1337,
				Gid:   1234,
				Sgids: defaultExecUser.Sgids,
				Home:  defaultExecUser.Home,
			},
		},
		{
			ref: "1337",
			expected: ExecUser{
				Uid:   1337,
				Gid:   defaultExecUser.Gid,
				Sgids: defaultExecUser.Sgids,
				Home:  defaultExecUser.Home,
			},
		},
		{
			ref: "",
			expected: ExecUser{
				Uid:   defaultExecUser.Uid,
				Gid:   defaultExecUser.Gid,
				Sgids: defaultExecUser.Sgids,
				Home:  defaultExecUser.Home,
			},
		},

		// Regression tests for #695.
		{
			ref: "111",
			expected: ExecUser{
				Uid:   111,
				Gid:   112,
				Sgids: defaultExecUser.Sgids,
				Home:  "/home/odd",
			},
		},
		{
			ref: "111:444",
			expected: ExecUser{
				Uid:   111,
				Gid:   444,
				Sgids: defaultExecUser.Sgids,
				Home:  "/home/odd",
			},
		},
		// Test for #3036.
		{
			ref: "7456",
			expected: ExecUser{
				Uid:   7456,
				Gid:   100,
				Sgids: []int{1234, 1000}, // 1000 is largegroup GID
				Home:  "/home/user7456",
			},
		},
		{
			ref: "7456:2147483647",
			expected: ExecUser{
				Uid:   7456,
				Gid:   2147483647, // maxID
				Sgids: defaultExecUser.Sgids,
				Home:  "/home/user7456",
			},
		},
		{
			ref: "2147483647:43",
			expected: ExecUser{
				Uid:   2147483647, // maxID
				Gid:   43,
				Sgids: defaultExecUser.Sgids,
				Home:  defaultExecUser.Home,
			},
		},
		{
			ref: "2147483647",
			expected: ExecUser{
				Uid:   2147483647, // maxID
				Gid:   defaultExecUser.Gid,
				Sgids: defaultExecUser.Sgids,
				Home:  defaultExecUser.Home,
			},
		},
	}

	for _, tc := range tests {
		name := tc.ref
		if name == "" {
			name = "<empty>"
		}
		t.Run(name, func(t *testing.T) {
			passwd := strings.NewReader(passwdContent)
			group := strings.NewReader(groupContent)

			execUser, err := GetExecUser(tc.ref, &defaultExecUser, passwd, group)
			if err != nil {
				t.Fatalf("got unexpected error when parsing '%s': %s", tc.ref, err.Error())
			}

			if !reflect.DeepEqual(tc.expected, *execUser) {
				t.Logf("ref:      %v", tc.ref)
				t.Logf("got:      %#v", execUser)
				t.Logf("expected: %#v", tc.expected)
				t.Fail()
			}
		})
	}
}

func TestGetExecUserInvalid(t *testing.T) {
	const passwdContent = `
root:x:0:0:root user:/root:/bin/bash
adm:x:42:43:adm:/var/adm:/bin/false
-42:x:12:13:broken:/very/broken
2147483647:x:0:0:maxint32:/root:/bin/bash
2147483648:x:0:0:toolarge:/root:/bin/bash
9223372036854775807:x:0:0:maxint64:/root:/bin/bash
9223372036854775808:x:0:0:maxint64plusone:/root:/bin/bash
this is just some garbage data
`
	const groupContent = `
root:x:0:root
adm:x:43:
grp:x:1234:root,adm
2147483647:x:1235:
2147483648:x:1236:
9223372036854775807:x:1237:
9223372036854775808:x:1238:
this is just some garbage data
`

	tests := []string{
		// No such user/group.
		"notuser",
		"notuser:notgroup",
		"root:notgroup",
		"notuser:adm",
		"8888:notgroup",
		"notuser:8888",

		// Invalid user/group values.
		"-1:0",
		"0:-3",
		"-5:-2",
		"-42",
		"-43",
		"42:2147483648",            // maxID + 1
		"2147483648:43",            // maxID + 1
		"2147483648",               // maxID + 1
		"7456:9223372036854775807", // maxInt64
		"9223372036854775807:43",   // maxInt64
		"9223372036854775807",      // maxInt64
		"9223372036854775808",      // maxInt64+1, must not resolve as username
		"0:9223372036854775808",    // maxInt64+1, must not resolve as group name
	}

	for _, tc := range tests {
		t.Run(tc, func(t *testing.T) {
			passwd := strings.NewReader(passwdContent)
			group := strings.NewReader(groupContent)

			execUser, err := GetExecUser(tc, nil, passwd, group)
			if err == nil {
				t.Fatalf("got unexpected success when parsing '%s': %#v", tc, execUser)
			}
		})
	}
}

func TestGetExecUserNilSources(t *testing.T) {
	const passwdContent = `
root:x:0:0:root user:/root:/bin/bash
adm:x:42:43:adm:/var/adm:/bin/false
this is just some garbage data
`
	const groupContent = `
root:x:0:root
adm:x:43:
grp:x:1234:root,adm
this is just some garbage data
`

	defaultExecUser := ExecUser{
		Uid:   8888,
		Gid:   8888,
		Sgids: []int{8888},
		Home:  "/8888",
	}

	tests := []struct {
		ref           string
		passwd, group bool
		expected      ExecUser
	}{
		{
			ref:    "",
			passwd: false,
			group:  false,
			expected: ExecUser{
				Uid:   8888,
				Gid:   8888,
				Sgids: []int{8888},
				Home:  "/8888",
			},
		},
		{
			ref:    "root",
			passwd: true,
			group:  false,
			expected: ExecUser{
				Uid:   0,
				Gid:   0,
				Sgids: []int{8888},
				Home:  "/root",
			},
		},
		{
			ref:    "0",
			passwd: false,
			group:  false,
			expected: ExecUser{
				Uid:   0,
				Gid:   8888,
				Sgids: []int{8888},
				Home:  "/8888",
			},
		},
		{
			ref:    "0:0",
			passwd: false,
			group:  false,
			expected: ExecUser{
				Uid:   0,
				Gid:   0,
				Sgids: []int{8888},
				Home:  "/8888",
			},
		},
	}

	for _, tc := range tests {
		name := tc.ref
		if name == "" {
			name = "<empty>"
		}
		t.Run(name, func(t *testing.T) {
			var passwd, group io.Reader

			if tc.passwd {
				passwd = strings.NewReader(passwdContent)
			}

			if tc.group {
				group = strings.NewReader(groupContent)
			}

			execUser, err := GetExecUser(tc.ref, &defaultExecUser, passwd, group)
			if err != nil {
				t.Fatalf("got unexpected error when parsing '%s': %s", tc.ref, err.Error())
			}

			if !reflect.DeepEqual(tc.expected, *execUser) {
				t.Logf("got:      %#v", execUser)
				t.Logf("expected: %#v", tc.expected)
				t.Fail()
			}
		})
	}
}

func TestGetAdditionalGroups(t *testing.T) {
	type foo struct {
		groups   []string
		expected []int
		hasError bool
	}

	groupContent := `
root:x:0:root
adm:x:43:
grp:x:1234:root,adm
adm:x:4343:root,adm-duplicate
2147483648:x:0:
9223372036854775808:x:0:
toolarge:x:2147483648:
this is just some garbage data
` + largeGroup()
	tests := []foo{
		{
			// empty group
			groups:   []string{},
			expected: []int{},
		},
		{
			// single group
			groups:   []string{"adm"},
			expected: []int{43},
		},
		{
			// numeric group miss must continue checking remaining groups
			groups:   []string{"10001", "adm"},
			expected: []int{43, 10001},
		},
		{
			// multiple groups
			groups:   []string{"adm", "grp"},
			expected: []int{43, 1234},
		},
		{
			// invalid group
			groups:   []string{"adm", "grp", "not-exist"},
			expected: nil,
			hasError: true,
		},
		{
			// group with numeric id
			groups:   []string{"43"},
			expected: []int{43},
		},
		{
			// group with unknown numeric id
			groups:   []string{"adm", "10001"},
			expected: []int{43, 10001},
		},
		{
			// groups specified twice with numeric and name
			groups:   []string{"adm", "43"},
			expected: []int{43},
		},
		{
			// groups with too small id
			groups:   []string{"-1"},
			expected: nil,
			hasError: true,
		},
		{
			// groups with too large id
			groups:   []string{strconv.FormatInt(1<<31, 10)},
			expected: nil,
			hasError: true,
		},
		{
			// group with very long list of users
			groups:   []string{"largegroup"},
			expected: []int{1000},
		},
		{
			// numeric group must not resolve as group name
			groups:   []string{"2147483648"},
			expected: nil,
			hasError: true,
		},
		{
			// numeric group must not resolve as group name
			groups:   []string{"9223372036854775808"},
			expected: nil,
			hasError: true,
		},
		{
			// group entry with out-of-range gid
			groups:   []string{"toolarge"},
			expected: nil,
			hasError: true,
		},
	}

	for _, tc := range tests {
		name := strings.Join(tc.groups, ",")
		t.Run(name, func(t *testing.T) {
			group := strings.NewReader(groupContent)

			gids, err := GetAdditionalGroups(tc.groups, group)
			if tc.hasError && err == nil {
				t.Fatalf("Parse(%#v) expects error but has none", tc)
			}
			if !tc.hasError && err != nil {
				t.Fatalf("Parse(%#v) has error %v", tc, err)
			}
			sort.Ints(gids)
			if !reflect.DeepEqual(gids, tc.expected) {
				t.Errorf("Gids(%v), expect %v from groups %v", gids, tc.expected, tc.groups)
			}
		})
	}
}

func TestGetAdditionalGroupsNumeric(t *testing.T) {
	tests := []struct {
		groups   []string
		expected []int
		hasError bool
	}{
		{
			// numeric groups only
			groups:   []string{"1234", "5678"},
			expected: []int{1234, 5678},
		},
		{
			// numeric and alphabetic
			groups:   []string{"1234", "fake"},
			expected: nil,
			hasError: true,
		},
	}

	for _, tc := range tests {
		name := strings.Join(tc.groups, ",")
		t.Run(name, func(t *testing.T) {
			gids, err := GetAdditionalGroups(tc.groups, nil)
			if tc.hasError && err == nil {
				t.Fatalf("Parse(%#v) expects error but has none", tc)
			}
			if !tc.hasError && err != nil {
				t.Fatalf("Parse(%#v) has error %v", tc, err)
			}
			sort.Ints(gids)
			if !reflect.DeepEqual(gids, tc.expected) {
				t.Errorf("Gids(%v), expect %v from groups %v", gids, tc.expected, tc.groups)
			}
		})
	}
}

// Generate a proper "largegroup" entry for group tests.
func largeGroup() (res string) {
	var b strings.Builder
	b.WriteString("largegroup:x:1000:user1")
	for i := 2; i <= 7500; i++ {
		_, _ = fmt.Fprintf(&b, ",user%d", i)
	}
	return b.String()
}
