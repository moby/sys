package reexec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
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
	if Init() {
		// Make sure we exit in case re-exec didn't os.Exit on its own.
		os.Exit(0)
	}
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
	testError := errors.New("test-error: the command was canceled")

	tests := []struct {
		doc        string
		cmdAndArgs []string
		cancel     bool
		expOut     string
		expError   error
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
			expError:   context.Canceled,
		},
		{
			doc:        "context timeout",
			cmdAndArgs: []string{testReExec3},
			expOut:     "Hello test-reexec3",
			expError:   testError,
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
			cmd.Cancel = func() error {
				return testError
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
			if !errors.Is(err, tc.expError) {
				t.Errorf("expected %v, got: %v", tc.expError, err)
			}

			actual := strings.TrimSpace(string(out))
			if actual != tc.expOut {
				t.Errorf("got %v, want %v", actual, tc.expOut)
			}
		})
	}
}

// TestRunNaiveSelf verifies that reexec.Self() (and thus CommandContext)
// can resolve a path that can be used to re-execute the current test binary
// when it falls back to the argv[0]-based implementation.
//
// It invokes the binary via naiveSelf (intentionally bypassing the Linux
// /proc/self/exe fast-path) so the fallback logic is exercised consistently
// across platforms.
func TestRunNaiveSelf(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Similar to [rexec.CommandContext], but using naiveSelf to skip the
	// optimized "/proc/self/exe" on Linux.
	cmd := exec.CommandContext(ctx, naiveSelf(os.Args[0]), testReExec2)
	cmd.Args = cmd.Args[1:]

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Unable to start command: %v", err)
	}

	expOut := "Hello test-reexec2"
	actual := strings.TrimSpace(string(out))
	if actual != expOut {
		t.Errorf("got %v, want %v", actual, expOut)
	}
}

func TestNaiveSelfResolve(t *testing.T) {
	t.Run("resolve in PATH", func(t *testing.T) {
		executable := "sh"
		if runtime.GOOS == "windows" {
			executable = "cmd"
		}
		resolved := naiveSelf(executable)
		if resolved == executable {
			t.Errorf("did not resolve via PATH; got %q", resolved)
		}
		if !filepath.IsAbs(resolved) {
			t.Errorf("expected absolute path; got %q", resolved)
		}
	})
	t.Run("not in PATH", func(t *testing.T) {
		const executable = "some-nonexistent-executable"
		want, err := filepath.Abs(executable)
		if err != nil {
			t.Fatal(err)
		}
		resolved := naiveSelf(executable)
		if resolved != want {
			t.Errorf("expected absolute path; got %q, want %q", resolved, want)
		}
	})
	t.Run("relative path", func(t *testing.T) {
		executable := filepath.Join(".", "some-executable")
		want, err := filepath.Abs(executable)
		if err != nil {
			t.Fatal(err)
		}
		resolved := naiveSelf(executable)
		if resolved != want {
			t.Errorf("expected absolute path; got %q, want %q", resolved, want)
		}
	})
	t.Run("absolute path unchanged", func(t *testing.T) {
		executable := filepath.Join(os.TempDir(), "some-executable")
		resolved := naiveSelf(executable)
		if resolved != executable {
			t.Errorf("should not modify absolute paths; got %q, want %q", resolved, executable)
		}
	})
}
