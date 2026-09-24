// SPDX-FileCopyrightText: 2025 The Go Authors
// SPDX-FileCopyrightText: 2025 The moby/sys Authors.
// SPDX-License-Identifier: Apache-2.0

// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.BSD file.

package uname_test

import (
	"testing"

	"github.com/moby/sys/uname"
)

func TestKernelVersionGE(t *testing.T) {
	major, minor := uname.KernelVersion()
	t.Logf("Running on kernel %d.%d", major, minor)

	tests := []struct {
		name string
		x, y int
		want bool
	}{
		{
			name: "current version equals itself",
			x:    major,
			y:    minor,
			want: true,
		},
		{
			name: "older major version",
			x:    major - 1,
			y:    0,
			want: true,
		},
		{
			name: "same major, older minor version",
			x:    major,
			y:    minor - 1,
			want: true,
		},
		{
			name: "newer major version",
			x:    major + 1,
			y:    0,
			want: false,
		},
		{
			name: "same major, newer minor version",
			x:    major,
			y:    minor + 1,
			want: false,
		},
		{
			name: "min version (0.0)",
			x:    0,
			y:    0,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uname.KernelVersionGE(tt.x, tt.y)
			if got != tt.want {
				t.Errorf("KernelVersionGE(%d, %d): got %v, want %v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}
