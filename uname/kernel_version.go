// SPDX-FileCopyrightText: 2025 The moby/sys Authors.
// SPDX-License-Identifier: Apache-2.0

// Package uname provides a simple way to check the kernel version.
// Currently it only supports Linux.
package uname

// KernelVersion returns major and minor kernel version numbers
// parsed from the [syscall.Uname] Release field, or (0, 0) if
// the version can't be obtained or parsed.
func KernelVersion() (major, minor int) {
	return kernelVersion()
}
