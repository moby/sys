// SPDX-FileCopyrightText: 2022 The Go Authors
// SPDX-FileCopyrightText: 2025 The moby/sys Authors.
// SPDX-License-Identifier: Apache-2.0

// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.BSD file.

//go:build !linux

package uname

func kernelVersion() (major int, minor int) {
	return 0, 0
}
