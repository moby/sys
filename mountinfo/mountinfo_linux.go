package mountinfo

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

// GetMountsFromReader retrieves a list of mounts from the
// reader provided, with an optional filter applied (use nil
// for no filter). This can be useful in tests or benchmarks
// that provide fake mountinfo data, or when a source other
// than /proc/thread-self/mountinfo needs to be read from.
//
// This function is Linux-specific.
func GetMountsFromReader(r io.Reader, filter FilterFunc) ([]*Info, error) {
	s := bufio.NewScanner(r)
	out := []*Info{}
	for s.Scan() {
		/*
		   See http://man7.org/linux/man-pages/man5/proc.5.html

		   36 35 98:0 /mnt1 /mnt2 rw,noatime master:1 - ext3 /dev/root rw,errors=continue
		   (1)(2)(3)   (4)   (5)      (6)      (7)   (8) (9)   (10)         (11)

		   (1) mount ID:  unique identifier of the mount (may be reused after umount)
		   (2) parent ID:  ID of parent (or of self for the top of the mount tree)
		   (3) major:minor:  value of st_dev for files on filesystem
		   (4) root:  root of the mount within the filesystem
		   (5) mount point:  mount point relative to the process's root
		   (6) mount options:  per mount options
		   (7) optional fields:  zero or more fields of the form "tag[:value]"
		   (8) separator:  marks the end of the optional fields
		   (9) filesystem type:  name of filesystem of the form "type[.subtype]"
		   (10) mount source:  filesystem specific information or "none"
		   (11) super options:  per super block options

		   In other words, we have:
		    * 6 mandatory fields	(1)..(6)
		    * 0 or more optional fields	(7)
		    * a separator field		(8)
		    * 3 mandatory fields	(9)..(11)
		*/

		text := s.Text()
		fields := strings.Split(text, " ")
		numFields := len(fields)
		if numFields < 10 {
			// should be at least 10 fields
			return nil, fmt.Errorf("parsing '%s' failed: not enough fields (%d)", text, numFields)
		}

		// separator field
		sepIdx := numFields - 4
		// In Linux <= 3.9 mounting a cifs with spaces in a share
		// name (like "//srv/My Docs") _may_ end up having a space
		// in the last field of mountinfo (like "unc=//serv/My Docs").
		// Since kernel 3.10-rc1, cifs option "unc=" is ignored,
		// so spaces should not appear.
		//
		// Check for a separator, and work around the spaces bug
		for fields[sepIdx] != "-" {
			sepIdx--
			if sepIdx == 5 {
				return nil, fmt.Errorf("parsing '%s' failed: missing - separator", text)
			}
		}

		major, minor, ok := strings.Cut(fields[2], ":")
		if !ok {
			return nil, fmt.Errorf("parsing '%s' failed: unexpected major:minor pair %s", text, fields[2])
		}

		p := &Info{
			ID:         toInt(fields[0]),
			Parent:     toInt(fields[1]),
			Major:      toInt(major),
			Minor:      toInt(minor),
			Root:       unescape(fields[3]),
			Mountpoint: unescape(fields[4]),
			Options:    fields[5],
			Optional:   strings.Join(fields[6:sepIdx], " "), // zero or more optional fields
			FSType:     unescape(fields[sepIdx+1]),
			Source:     unescape(fields[sepIdx+2]),
			VFSOptions: fields[sepIdx+3],
		}

		// Run the filter after parsing all fields.
		var skip, stop bool
		if filter != nil {
			skip, stop = filter(p)
			if skip {
				continue
			}
		}

		out = append(out, p)
		if stop {
			break
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

var (
	haveProcThreadSelf     bool
	haveProcThreadSelfOnce sync.Once
)

func parseMountTable(filter FilterFunc) (_ []*Info, err error) {
	haveProcThreadSelfOnce.Do(func() {
		_, err := os.Stat("/proc/thread-self/mountinfo")
		haveProcThreadSelf = err == nil
	})

	// We need to lock ourselves to the current OS thread in order to make sure
	// that the thread referenced by /proc/thread-self stays alive until we
	// finish parsing the file.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var f *os.File
	if haveProcThreadSelf {
		f, err = os.Open("/proc/thread-self/mountinfo")
	} else {
		// On pre-3.17 kernels (such as CentOS 7), we don't have
		// /proc/thread-self/ so we need to manually construct
		// /proc/self/task/<tid>/ as a fallback.
		f, err = os.Open("/proc/self/task/" + strconv.Itoa(unix.Gettid()) + "/mountinfo")
		if os.IsNotExist(err) {
			// If /proc/self/task/... failed, it means that our active pid
			// namespace doesn't match the pid namespace of the /proc mount. In
			// this case we just have to make do with /proc/self, since there
			// is no other way of figuring out our tid in a parent pid
			// namespace on pre-3.17 kernels.
			f, err = os.Open("/proc/self/mountinfo")
		}
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return GetMountsFromReader(f, filter)
}

// PidMountInfo retrieves the list of mounts from a given process' mount
// namespace. Unless there is a need to get mounts from a mount namespace
// different from that of a calling process, use GetMounts.
//
// This function is Linux-specific.
//
// Deprecated: this will be removed before v1; use GetMountsFromReader with
// opened /proc/<pid>/mountinfo as an argument instead.
func PidMountInfo(pid int) ([]*Info, error) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/mountinfo", pid))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return GetMountsFromReader(f, nil)
}

// Linux /proc/mounts shows current mounts.
// Same format as /etc/fstab. Quoting getmntent(3):
//
// Since fields in the mtab and fstab files are separated by whitespace,
// octal escapes are used to represent the four characters space (\040),
// tab (\011), newline (\012) and backslash (\134) in those files when
// they occur in one of the four strings in a mntent structure.
//
// http://linux.die.net/man/3/getmntent
var fstabUnescape = strings.NewReplacer(
	`\040`, "\040",
	`\011`, "\011",
	`\012`, "\012",
	`\134`, "\134",
)

// A few specific characters in mountinfo path entries (root and mountpoint)
// are escaped using a backslash followed by a character's ascii code in octal.
//
//	space              -- as \040
//	tab (aka \t)       -- as \011
//	newline (aka \n)   -- as \012
//	backslash (aka \\) -- as \134
//
// This function converts path from mountinfo back, i.e. it unescapes the above sequences.
func unescape(path string) string {
	// try to avoid copying
	if strings.IndexByte(path, '\\') == -1 {
		return path
	}

	return fstabUnescape.Replace(path)
}

// toInt converts a string to an int, and ignores any numbers parsing errors,
// as there should not be any.
func toInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}
