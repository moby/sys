// SPDX-FileCopyrightText: 2025 The moby/sys Authors.
// SPDX-License-Identifier: Apache-2.0

package uname_test

import (
	"runtime"
	"testing"

	"github.com/moby/sys/uname"
)

func TestKernelVersion(t *testing.T) {
	x, y := uname.KernelVersion()
	t.Logf("KernelVersion: %d.%d (GOOS: %s)", x, y, runtime.GOOS)
	switch runtime.GOOS {
	case "linux":
		// Go requires Linux >= 2.x.
		if x < 2 {
			t.Errorf("want major >= 2, got %d", x)
		}
		// Sanity check.
		if y < 0 {
			t.Errorf("want minor >= 0, got %d", y)
		}
	default:
		if x != 0 || y != 0 {
			t.Fatalf("want 0.0, got %d.%d", x, y)
		}
	}
}
