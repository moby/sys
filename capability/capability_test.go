// Copyright 2023 The Capability Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package capability

import (
	"runtime"
	"testing"
)

// Based on the fact Go 1.18+ supports Linux >= 2.6.32, and
//   - CAP_MAC_ADMIN (33) was added in 2.6.25;
//   - CAP_SYSLOG (34) was added in 2.6.38;
//   - CAP_CHECKPOINT_RESTORE (40) was added in 5.9, and it is
//     the last added capability as of today (July 2024);
//
// LastCap return value should be between minLastCap and maxLastCap.
const (
	minLastCap = CAP_MAC_ADMIN
	maxLastCap = CAP_CHECKPOINT_RESTORE
)

func requirePCapSet(t *testing.T) {
	pid, err := NewPid(0)
	if err != nil {
		t.Fatal(err)
	}
	if !pid.Get(EFFECTIVE, CAP_SETPCAP) {
		t.Skip("The test needs `CAP_SETPCAP`.")
	}
}

func TestLastCap(t *testing.T) {
	last, err := LastCap()
	switch runtime.GOOS {
	case "linux":
		if err != nil {
			t.Fatal(err)
		}
	default:
		if err == nil {
			t.Fatal(runtime.GOOS, ": want error, got nil")
		}
		return
	}

	// Sanity checks (Linux only).
	if last < minLastCap {
		t.Fatalf("LastCap returned %d (%s), expected >= %d (%s)",
			last, last, minLastCap, minLastCap)
	}
	if last > maxLastCap {
		t.Fatalf("LastCap returned %d, expected <= %d (%s). Package needs to be updated.",
			last, maxLastCap, maxLastCap)
	}
}

func TestListSupported(t *testing.T) {
	list, err := ListSupported()
	switch runtime.GOOS {
	case "linux":
		if err != nil {
			t.Fatal(err)
		}
	default:
		if err == nil {
			t.Fatal(runtime.GOOS, ": want error, got nil")
		}
	}
	if runtime.GOOS != "linux" {
		return
	}
	// Sanity check (Linux only).
	t.Logf("got +%v (len %d)", list, len(list))
	minLen := int(minLastCap) + 1
	if len(list) < minLen {
		t.Fatalf("result is too short (got %d, want %d): +%v", len(list), minLen, list)
	}
}

func TestAmbientCapSet(t *testing.T) {
	if runtime.GOOS != "linux" {
		return
	}
	requirePCapSet(t)

	capBounding := []Cap{CAP_KILL, CAP_CHOWN, CAP_SYSLOG}
	capPermitted := []Cap{CAP_KILL, CAP_CHOWN}
	capEffective := []Cap{CAP_KILL}
	capInheritable := []Cap{CAP_KILL, CAP_CHOWN}
	capAmbient := []Cap{CAP_KILL, CAP_CHOWN}

	pid, err := newPid(0)
	if err != nil {
		t.Fatal(err)
	}
	pid.Set(BOUNDING, capBounding...)
	pid.Set(PERMITTED, capPermitted...)
	pid.Set(EFFECTIVE, capEffective...)
	pid.Set(INHERITABLE, capInheritable...)
	pid.Set(AMBIENT, capAmbient...)
	if err = pid.Apply(CAPS | BOUNDING | AMBIENT); err != nil {
		t.Fatal(err)
	}

	// Restore the cap set data from current process
	if err = pid.Load(); err != nil {
		t.Fatal(err)
	}
	for _, cap := range capAmbient {
		if !pid.Get(AMBIENT, cap) {
			t.Fatalf("expected ambient cap(%d) to be set but it's not", cap)
		}
	}

	// Remove a ambient cap, to check `PR_CAP_AMBIENT_CLEAR_ALL` work.
	pid.Clear(AMBIENT)
	pid.Set(AMBIENT, capAmbient[0])
	if err = pid.Apply(CAPS | BOUNDING | AMBIENT); err != nil {
		t.Fatal(err)
	}

	if err = pid.Load(); err != nil {
		t.Fatal(err)
	}
	if !pid.Get(AMBIENT, capAmbient[0]) {
		t.Fatalf("expected ambient cap(%d) to be set but it's not", capAmbient[0])
	}
	if pid.Get(AMBIENT, capAmbient[1]) {
		t.Fatalf("expected ambient cap(%d) not to be set but it has been set", capAmbient[1])
	}
}
