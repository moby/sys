//go:build !windows

// SPDX-License-Identifier: Apache-2.0
/*
 * Copyright (C) 2015-2026 Open Containers Initiative Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// This code originally comes from runc and was taken from this tree:
// <https://github.com/opencontainers/runc/tree/v1.4.0/libcontainer/devices>.

package devices

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/opencontainers/cgroups/devices/config"
	"golang.org/x/sys/unix"
)

func cleanupTest() {
	unixLstat = unix.Lstat
	osReadDir = os.ReadDir
}

func TestDeviceFromPathLstatFailure(t *testing.T) {
	testError := errors.New("test error")

	// Override unix.Lstat to inject error.
	unixLstat = func(path string, stat *unix.Stat_t) error {
		return testError
	}
	defer cleanupTest()

	_, err := DeviceFromPath("", "")
	if !errors.Is(err, testError) {
		t.Fatalf("Unexpected error %v, expected %v", err, testError)
	}
}

func TestHostDevicesIoutilReadDirFailure(t *testing.T) {
	testError := errors.New("test error")

	// Override os.ReadDir to inject error.
	osReadDir = func(dirname string) ([]fs.DirEntry, error) {
		return nil, testError
	}
	defer cleanupTest()

	_, err := HostDevices()
	if !errors.Is(err, testError) {
		t.Fatalf("Unexpected error %v, expected %v", err, testError)
	}
}

func TestHostDevicesIoutilReadDirDeepFailure(t *testing.T) {
	testError := errors.New("test error")
	called := false

	// Override os.ReadDir to inject error after the first call.
	osReadDir = func(dirname string) ([]fs.DirEntry, error) {
		if called {
			return nil, testError
		}
		called = true

		// Provoke a second call.
		fi, err := os.Stat("/tmp")
		if err != nil {
			t.Fatalf("Unexpected error %v", err)
		}

		return []fs.DirEntry{fs.FileInfoToDirEntry(fi)}, nil
	}
	defer cleanupTest()

	_, err := HostDevices()
	if !errors.Is(err, testError) {
		t.Fatalf("Unexpected error %v, expected %v", err, testError)
	}
}

func TestHostDevicesAllValid(t *testing.T) {
	devices, err := HostDevices()
	if err != nil {
		t.Fatalf("failed to get host devices: %v", err)
	}

	for _, device := range devices {
		// Devices can't have major number 0 on Linux.
		if device.Major == 0 {
			logFn := t.Logf
			if runtime.GOOS == "linux" {
				logFn = t.Errorf
			}
			logFn("device entry %+v has zero major number", device)
		}
		switch device.Type {
		case config.BlockDevice, config.CharDevice:
		case config.FifoDevice:
			t.Logf("fifo devices shouldn't show up from HostDevices")
			fallthrough
		default:
			t.Errorf("device entry %+v has unexpected type %v", device, device.Type)
		}
	}
}

type namedDir struct {
	name string
}

func (d namedDir) Name() string               { return d.name }
func (d namedDir) IsDir() bool                { return true }
func (d namedDir) Type() fs.FileMode          { return fs.ModeDir }
func (d namedDir) Info() (fs.FileInfo, error) { return nil, errors.New("unused") }

func TestGetDevicesSkipsIncusMounts(t *testing.T) {
	t.Cleanup(cleanupTest)

	for _, tc := range []struct {
		name    string
		wantErr bool
	}{
		{name: ".incus-mounts"},
		{name: "private", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			osReadDir = func(dirname string) ([]fs.DirEntry, error) {
				if filepath.Base(dirname) == tc.name {
					return nil, os.ErrPermission
				}
				return []fs.DirEntry{namedDir{name: tc.name}}, nil
			}

			_, err := GetDevices("/dev")
			if tc.wantErr {
				if !errors.Is(err, os.ErrPermission) {
					t.Fatalf("got %v, want permission", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetDevices: %v", err)
			}
		})
	}
}
