// Copyright 2023 The Capability Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package capability

import (
	"runtime"
	"testing"
)

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
	//
	// Based on the fact Go 1.18+ supports Linux >= 2.6.32, and
	//   - CAP_MAC_ADMIN (33) was added in 2.6.25;
	//   - CAP_SYSLOG (34) was added in 2.6.38;
	//   - CAP_CHECKPOINT_RESTORE (40) was added in 5.9, and it is
	//     the last added capability as of today (July 2024);
	// LastCap return value should be between minCap and maxCap.
	minCap := CAP_MAC_ADMIN
	maxCap := CAP_CHECKPOINT_RESTORE
	if last < minCap {
		t.Fatalf("LastCap returned %d (%s), expected >= %d (%s)",
			last, last, minCap, minCap)
	}
	if last > maxCap {
		t.Fatalf("LastCap returned %d, expected <= %d (%s). Package needs to be updated.",
			last, maxCap, maxCap)
	}
}
