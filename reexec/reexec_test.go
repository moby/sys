package reexec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	testReExec  = "test-reexec"
	testReExec2 = "test-reexec2"
	testReExec3 = "test-reexec3"
)

func init() {
	Register(testReExec, func() {
		panic("Return Error")
	})
	Register(testReExec2, func() {
		var args string
		if len(os.Args) > 1 {
			args = fmt.Sprintf("(args: %#v)", os.Args[1:])
		}
		fmt.Println("Hello", testReExec2, args)
		os.Exit(0)
	})
	Register(testReExec3, func() {
		fmt.Println("Hello " + testReExec3)
		time.Sleep(1 * time.Second)
		os.Exit(0)
	})
	Init()
}

func TestRegister(t *testing.T) {
	tests := []struct {
		doc         string
		name        string
		expectedErr string
	}{
		{
			doc:         "duplicate name",
			name:        testReExec,
			expectedErr: `reexec func already registered under name "test-reexec"`,
		},
		{
			doc:         "invalid name",
			name:        filepath.Join("something", testReExec),
			expectedErr: fmt.Sprintf("reexec func does not expect a path component: %q", filepath.Join("something", testReExec)),
		},
	}

	for _, tc := range tests {
		t.Run(tc.doc, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("Register() did not panic")
					return
				}
				if r != tc.expectedErr {
					t.Errorf("got %q, want %q", r, tc.expectedErr)
				}
			}()
			Register(tc.name, func() {})
		})
	}
}

func TestCommand(t *testing.T) {
	tests := []struct {
		doc        string
		cmdAndArgs []string
		expOut     string
	}{
		{
			doc:        "basename",
			cmdAndArgs: []string{testReExec2},
			expOut:     "Hello test-reexec2",
		},
		{
			doc:        "full path",
			cmdAndArgs: []string{filepath.Join("something", testReExec2)},
			expOut:     `Hello test-reexec2`,
		},
		{
			doc:        "command with args",
			cmdAndArgs: []string{testReExec2, "--some-flag", "some-value", "arg1", "arg2"},
			expOut:     `Hello test-reexec2 (args: []string{"--some-flag", "some-value", "arg1", "arg2"})`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.doc, func(t *testing.T) {
			cmd := Command(tc.cmdAndArgs...)
			if !reflect.DeepEqual(cmd.Args, tc.cmdAndArgs) {
				t.Fatalf("got %+v, want %+v", cmd.Args, tc.cmdAndArgs)
			}
			w, err := cmd.StdinPipe()
			if err != nil {
				t.Fatalf("Error on pipe creation: %v", err)
			}
			defer func() { _ = w.Close() }()

			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("Error on re-exec cmd: %v, out: %v", err, string(out))
			}

			actual := strings.TrimSpace(string(out))
			if actual != tc.expOut {
				t.Errorf("got %v, want %v", actual, tc.expOut)
			}
		})
	}
}

func TestCommandContext(t *testing.T) {
	tests := []struct {
		doc        string
		cmdAndArgs []string
		cancel     bool
		expOut     string
		expError   bool
	}{
		{
			doc:        "basename",
			cmdAndArgs: []string{testReExec2},
			expOut:     "Hello test-reexec2",
		},
		{
			doc:        "full path",
			cmdAndArgs: []string{filepath.Join("something", testReExec2)},
			expOut:     "Hello test-reexec2",
		},
		{
			doc:        "command with args",
			cmdAndArgs: []string{testReExec2, "--some-flag", "some-value", "arg1", "arg2"},
			expOut:     `Hello test-reexec2 (args: []string{"--some-flag", "some-value", "arg1", "arg2"})`,
		},
		{
			doc:        "context canceled",
			cancel:     true,
			cmdAndArgs: []string{testReExec2},
			expError:   true,
		},
		{
			doc:        "context timeout",
			cmdAndArgs: []string{testReExec3},
			expOut:     "Hello test-reexec3",
			expError:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.doc, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			cmd := CommandContext(ctx, tc.cmdAndArgs...)
			if !reflect.DeepEqual(cmd.Args, tc.cmdAndArgs) {
				t.Fatalf("got %+v, want %+v", cmd.Args, tc.cmdAndArgs)
			}

			w, err := cmd.StdinPipe()
			if err != nil {
				t.Fatalf("Error on pipe creation: %v", err)
			}
			defer func() { _ = w.Close() }()
			if tc.cancel {
				cancel()
			}
			out, err := cmd.CombinedOutput()
			if tc.cancel {
				if !errors.Is(err, context.Canceled) {
					t.Errorf("got %[1]v (%[1]T), want %v", err, context.Canceled)
				}
			}
			if tc.expError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else if err != nil {
				t.Errorf("error on re-exec cmd: %v, out: %v", err, string(out))
			}

			actual := strings.TrimSpace(string(out))
			if actual != tc.expOut {
				t.Errorf("got %v, want %v", actual, tc.expOut)
			}
		})
	}
}

func TestNaiveSelf(t *testing.T) {
	if os.Getenv("TEST_CHECK") == "1" {
		os.Exit(2)
	}
	cmd := exec.Command(naiveSelf(), "-test.run=TestNaiveSelf")
	cmd.Env = append(os.Environ(), "TEST_CHECK=1")
	err := cmd.Start()
	if err != nil {
		t.Fatalf("Unable to start command: %v", err)
	}
	err = cmd.Wait()

	var expError *exec.ExitError
	if !errors.As(err, &expError) {
		t.Fatalf("got %T, want %T", err, expError)
	}

	const expected = "exit status 2"
	if err.Error() != expected {
		t.Fatalf("got %v, want %v", err, expected)
	}

	os.Args[0] = "mkdir"
	if naiveSelf() == os.Args[0] {
		t.Fatalf("Expected naiveSelf to resolve the location of mkdir")
	}
}
