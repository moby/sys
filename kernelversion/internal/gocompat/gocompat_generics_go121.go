// SPDX-License-Identifier: BSD-3-Clause

//go:build linux && go1.21

// Copyright (C) 2024-2025 SUSE LLC. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gocompat

import (
	"cmp"
	"sync"
)

// SyncOnceValues is equivalent to Go 1.21's sync.OnceValues.
func SyncOnceValues[T1, T2 any](f func() (T1, T2)) func() (T1, T2) {
	return sync.OnceValues(f)
}

// CmpOrdered is equivalent to Go 1.21's cmp.Ordered generic type definition.
type CmpOrdered = cmp.Ordered

// CmpCompare is equivalent to Go 1.21's cmp.Compare.
func CmpCompare[T CmpOrdered](x, y T) int {
	return cmp.Compare(x, y)
}

// Max2 is equivalent to Go 1.21's max builtin (but only for two parameters).
func Max2[T CmpOrdered](x, y T) T {
	return max(x, y)
}
