// Copyright 2023 The Capability Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package capability_test

import (
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"

	. "github.com/moby/sys/capability"
)

func init() {
	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		// Lock the main goroutine to the main thread, so that tests
		// (which run in separate goroutines) never run on the main
		// thread. This ensures per-thread capabilities are handled
		// properly.
		runtime.LockOSThread()
	}
}

func requirePCapSet(t testing.TB) {
	t.Helper()
	pid, err := NewPid2(0)
	if err != nil {
		t.Fatal(err)
	}
	if err := pid.Load(); err != nil {
		t.Fatal(err)
	}
	if !pid.Get(EFFECTIVE, CAP_SETPCAP) {
		t.Skip("The test needs `CAP_SETPCAP`.")
	}
}

// testInChild runs fn as a separate process, and returns its output.
// This is useful for tests which manipulate capabilities, allowing to
// preserve those of the main test process.
//
// The fn is a function which must end with os.Exit. In case exit code
// is non-zero, t.Fatal is called.
func testInChild(t *testing.T, fn func()) []byte {
	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		fn()
	}

	// Re-exec the current test as a new process.
	args := []string{"-test.run=^" + t.Name() + "$"}
	if testing.Verbose() {
		args = append(args, "-test.v")
	}
	cmd := exec.Command("/proc/self/exe", args...)
	cmd.Env = append(cmd.Environ(), "GO_WANT_HELPER_PROCESS=1")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Helper()
		t.Fatalf("exec failed: %v\n\n%s\n", err, out)
	}

	return out
}

func TestLastCap(t *testing.T) {
	last, err := LastCap()
	switch runtime.GOOS {
	default:
		if err == nil {
			t.Fatal(runtime.GOOS, ": want error, got nil")
		}
		return
	case "linux":
	}
	if err != nil {
		t.Fatalf("LastCap: want nil, got error: %v", err)
	}
	buf, err := os.ReadFile("/proc/sys/kernel/cap_last_cap")
	if err != nil {
		t.Skip(err)
	}
	want, err := strconv.Atoi(strings.TrimSpace(string(buf)))
	if err != nil {
		t.Fatal(err)
	}
	if int(last) != want {
		t.Errorf("LastCap: want %d (from /proc), got %d", want, last)
	}
}

func TestListSupported(t *testing.T) {
	list, err := ListSupported()
	switch runtime.GOOS {
	default:
		if err == nil {
			t.Fatal(runtime.GOOS, ": want error, got nil")
		}
		return
	case "linux":
	}
	if err != nil {
		t.Fatalf("ListSupported: want nil, got error: %v", err)
	}
	t.Logf("got +%v (len %d)", list, len(list))
	last, err := LastCap()
	if err != nil {
		t.Fatalf("LastCap: %v", err)
	}
	wantLen := min(int(last)+1, len(ListKnown()))
	if len(list) != wantLen {
		t.Errorf("ListSupported: want len %d, got %d: +%v", wantLen, len(list), list)
	}
}

func TestNewPid2Load(t *testing.T) {
	c, err := NewPid2(0)
	switch runtime.GOOS {
	default:
		if err == nil {
			t.Fatal(runtime.GOOS, ": want error, got nil")
		}
		return
	case "linux":
	}
	if err != nil {
		t.Fatalf("NewPid2: want nil, got error: %v", err)
	}
	err = c.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Assuming that at least bounding set is not empty.
	bset := c.StringCap(BOUNDING)
	t.Logf("Bounding set: %s", bset)
	if len(bset) == 0 {
		t.Fatal("loaded bounding set: want non-empty, got empty")
	}
}

func TestAmbientCapSet(t *testing.T) {
	if runtime.GOOS != "linux" {
		return
	}
	requirePCapSet(t)

	out := testInChild(t, childAmbientCapSet)

	t.Logf("output from child:\n%s", out)
}

func childAmbientCapSet() {
	runtime.LockOSThread()
	// We can't use t.Log etc. here, yet filename and line number is nice
	// to have. Set up and use the standard logger for this.
	log.SetFlags(log.Lshortfile)

	pid, err := NewPid2(0)
	if err != nil {
		log.Fatal(err)
	}

	list := []Cap{CAP_KILL, CAP_CHOWN, CAP_SYS_CHROOT}
	pid.Set(CAPS|AMBIENT, list...)
	if err = pid.Apply(CAPS | AMBIENT); err != nil {
		log.Fatal(err)
	}

	// Check if ambient caps were applied.
	if err = pid.Load(); err != nil {
		log.Fatal(err)
	}
	for _, cap := range list {
		want := true
		if got := pid.Get(AMBIENT, cap); want != got {
			log.Fatalf("Get(AMBIENT, %s): want %v, got %v", cap, want, got)
		}
	}

	// Unset a single ambient cap, to check `PR_CAP_AMBIENT_CLEAR_ALL` work.
	const unsetIdx = 1
	pid.Unset(AMBIENT, list[unsetIdx])
	if err = pid.Apply(AMBIENT); err != nil {
		log.Fatal(err)
	}

	if err = pid.Load(); err != nil {
		log.Fatal(err)
	}
	for i, cap := range list {
		want := i != unsetIdx
		if got := pid.Get(AMBIENT, cap); want != got {
			log.Fatalf("Get(AMBIENT, %s): want %v, got %v", cap, want, got)
		}
	}
	os.Exit(0)
}

// https://github.com/moby/sys/issues/168
func TestApplyOtherProcess(t *testing.T) {
	if runtime.GOOS != "linux" {
		return
	}
	requirePCapSet(t)

	cmd := exec.Command("sleep", "infinity")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	pid, err := NewPid2(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	pid.Clear(CAPS | BOUNDS | AMBS)

	// See (*capsV3).Apply.
	expErr := "unable to modify capabilities of another process"

	for _, arg := range []CapType{CAPS, BOUNDS, AMBS} {
		err = pid.Apply(arg)
		if !strings.Contains(err.Error(), expErr) {
			t.Errorf("Apply(%q): want error to contain %q; got %v", arg, expErr, err)
		}
	}
}

func TestGetSetResetAmbient(t *testing.T) {
	if runtime.GOOS != "linux" {
		_, err := GetAmbient(Cap(0))
		if err == nil {
			t.Error(runtime.GOOS, ": want error, got nil")
		}
		err = SetAmbient(false, Cap(0))
		if err == nil {
			t.Error(runtime.GOOS, ": want error, got nil")
		}
		err = ResetAmbient()
		if err == nil {
			t.Error(runtime.GOOS, ": want error, got nil")
		}
		return
	}

	requirePCapSet(t)
	out := testInChild(t, childGetSetResetAmbient)
	t.Logf("output from child:\n%s", out)
}

func childGetSetResetAmbient() {
	runtime.LockOSThread()
	log.SetFlags(log.Lshortfile)

	pid, err := NewPid2(0)
	if err != nil {
		log.Fatal(err)
	}

	list := []Cap{CAP_KILL, CAP_CHOWN, CAP_SYS_CHROOT}
	pid.Set(CAPS, list...)
	if err = pid.Apply(CAPS); err != nil {
		log.Fatal(err)
	}

	// Set ambient caps from list.
	if err = SetAmbient(true, list...); err != nil {
		log.Fatal(err)
	}

	// Check if they were set as expected.
	for _, cap := range list {
		want := true
		got, err := GetAmbient(cap)
		if err != nil {
			log.Fatalf("GetAmbient(%s): want nil, got error %v", cap, err)
		} else if want != got {
			log.Fatalf("Get(AMBIENT, %s): want %v, got %v", cap, want, got)
		}
	}

	// Lower one ambient cap.
	const unsetIdx = 1
	if err = SetAmbient(false, list[unsetIdx]); err != nil {
		log.Fatal(err)
	}
	// Check they are set as expected.
	for i, cap := range list {
		want := i != unsetIdx
		got, err := GetAmbient(cap)
		if err != nil {
			log.Fatalf("GetAmbient(%s): want nil, got error %v", cap, err)
		} else if want != got {
			log.Fatalf("Get(AMBIENT, %s): want %v, got %v", cap, want, got)
		}
	}

	// Lower all ambient caps.
	if err = ResetAmbient(); err != nil {
		log.Fatal(err)
	}
	for _, cap := range list {
		want := false
		got, err := GetAmbient(cap)
		if err != nil {
			log.Fatalf("GetAmbient(%s): want nil, got error %v", cap, err)
		} else if want != got {
			log.Fatalf("Get(AMBIENT, %s): want %v, got %v", cap, want, got)
		}
	}
	os.Exit(0)
}

func TestGetBound(t *testing.T) {
	if runtime.GOOS != "linux" {
		_, err := GetBound(Cap(0))
		if err == nil {
			t.Error(runtime.GOOS, ": want error, got nil")
		}
		return
	}

	last, err := LastCap()
	if err != nil {
		t.Fatalf("LastCap: %v", err)
	}
	for i := range Cap(63) {
		wantErr := i > last
		set, err := GetBound(i)
		t.Logf("GetBound(%q): %v, %v", i, set, err)
		if wantErr && err == nil {
			t.Errorf("GetBound(%q): want err, got nil", i)
		} else if !wantErr && err != nil {
			t.Errorf("GetBound(%q): want nil, got error %v", i, err)
		}
	}
}

func TestDropBound(t *testing.T) {
	if runtime.GOOS != "linux" {
		err := DropBound(Cap(0))
		if err == nil {
			t.Error(runtime.GOOS, ": want error, got nil")
		}
		return
	}

	requirePCapSet(t)
	out := testInChild(t, childDropBound)
	t.Logf("output from child:\n%s", out)
}

func childDropBound() {
	runtime.LockOSThread()
	log.SetFlags(log.Lshortfile)

	for i := Cap(2); i < Cap(4); i++ {
		err := DropBound(i)
		if err != nil {
			log.Fatalf("DropBound(%q): want nil, got error %v", i, err)
		}
		set, err := GetBound(i)
		if err != nil {
			log.Fatalf("GetBound(%q): want nil, got error %v", i, err)
		}
		if set {
			log.Fatalf("GetBound(%q): want false, got true", i)
		}
	}

	os.Exit(0)
}
