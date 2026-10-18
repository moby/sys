// Copyright 2026 The Capability Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package capability

import (
	"path/filepath"
	"testing"
)

func TestLastCapFallsBackWhenProcFileIsMissing(t *testing.T) {
	want, err := LastCap()
	if err != nil {
		t.Fatal(err)
	}

	got, err := readLastCap(filepath.Join(t.TempDir(), "cap_last_cap"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("LastCap fallback = %d, want %d", got, want)
	}
}
