package reexec_test

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

	"github.com/moby/sys/reexec"
	"github.com/moby/sys/reexec/internal/reexecoverride"
)

const (
	testReExec  = "test-reexec"
	testReExec2 = "test-reexec2"
	testReExec3 = "test-reexec3"
)

var errTestEntrypointFailure = errors.New("test-reexec: simulated failure")

func init() {
	reexec.RegisterContext(testReExec, func(context.Context) error {
		return errTestEntrypointFailure
	})
	reexec.RegisterContext(testReExec2, func(context.Context) error {
		var args string
		if len(os.Args) > 1 {
			args = fmt.Sprintf("(args: %#v)", os.Args[1:])
		}
		fmt.Println("Hello", testReExec2, args)
		return nil
	})
	reexec.Register(testReExec3, func() {
		fmt.Println("Hello " + testReExec3)
		time.Sleep(1 * time.Second)
		os.Exit(0)
	})

	if ok, err := reexec.Dispatch(context.Background()); ok {
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "entrypoint failed:", err)
			os.Exit(1)
		}
		// Make sure we exit in case a re-exec entrypoint didn't os.Exit on its own.
		os.Exit(0)
	}
}

func TestRegister(t *testing.T) {
	type registerFunc struct {
		doc string
		fn  func(name string)
	}

	registerFuncs := []registerFunc{
		{
			doc: "Register",
			fn: func(name string) {
				reexec.Register(name, func() {})
			},
		},
		{
			doc: "RegisterContext",
			fn: func(name string) {
				reexec.RegisterContext(name, func(context.Context) error { return nil })
			},
		},
	}

	tests := []struct {
		doc         string
		name        string
		expectedErr string
	}{
		{
			doc:  "new entrypoint",
			name: "test-register-new",
		},
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

	for _, rf := range registerFuncs {
		for _, tc := range tests {
			t.Run(rf.doc+"/"+tc.doc, func(t *testing.T) {
				name := tc.name
				if tc.doc == "new entrypoint" {
					// Use the test name as a unique suffix; replace path separators.
					name = tc.name + "-" + strings.ReplaceAll(t.Name(), "/", "-")
				}

				defer func() {
					r := recover()
					switch {
					case tc.expectedErr == "":
						if r != nil {
							t.Fatalf("unexpected panic: %v", r)
						}
					case r == nil:
						t.Fatalf("expected panic %q, got nil", tc.expectedErr)
					case r != tc.expectedErr:
						t.Fatalf("got %q, want %q", r, tc.expectedErr)
					}
				}()

				rf.fn(name)
			})
		}
	}
}

func TestCommand(t *testing.T) {
	tests := []struct {
		doc        string
		cmdAndArgs []string
		expOut     string
		expExit    int
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
		{
			doc:        "entrypoint returns error",
			cmdAndArgs: []string{testReExec},
			expOut:     "entrypoint failed: " + errTestEntrypointFailure.Error(),
			expExit:    1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.doc, func(t *testing.T) {
			cmd := reexec.Command(tc.cmdAndArgs...)
			if !reflect.DeepEqual(cmd.Args, tc.cmdAndArgs) {
				t.Fatalf("got %+v, want %+v", cmd.Args, tc.cmdAndArgs)
			}
			w, err := cmd.StdinPipe()
			if err != nil {
				t.Fatalf("Error on pipe creation: %v", err)
			}
			defer func() { _ = w.Close() }()

			out, err := cmd.CombinedOutput()
			switch {
			case tc.expExit != 0:
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
				}
				if code := exitErr.ExitCode(); code != tc.expExit {
					t.Errorf("got exit code %d, want %d; out: %v", code, tc.expExit, string(out))
				}
			default:
				if err != nil {
					t.Errorf("unexpected error: %v (out: %v)", err, string(out))
				}
			}

			actual := strings.TrimSpace(string(out))
			if actual != tc.expOut {
				t.Errorf("got %v, want %v", actual, tc.expOut)
			}
		})
	}
}

func TestDispatch(t *testing.T) {
	type ctxKey struct{}
	const want = "sentinel"

	reexec.RegisterContext("test-dispatch", func(ctx context.Context) error {
		got, _ := ctx.Value(ctxKey{}).(string)
		if got != want {
			return fmt.Errorf("unexpected context value: got %q, want %q", got, want)
		}
		return nil
	})
	reexec.RegisterContext("test-dispatch-canceled", func(ctx context.Context) error {
		return ctx.Err()
	})
	reexec.Register("test-dispatch-no-ctx", func() {})

	tests := []struct {
		name  string
		ctx   func(t *testing.T) context.Context
		check func(t *testing.T, ok bool, err error)
	}{
		{
			name: "not-registered",
			ctx: func(t *testing.T) context.Context {
				return context.Background()
			},
			check: func(t *testing.T, ok bool, err error) {
				t.Helper()
				if ok {
					t.Errorf("expected ok=false, got true")
				}
				if err != nil {
					t.Errorf("expected err=nil, got %v", err)
				}
			},
		},
		{
			name: "test-dispatch",
			ctx: func(t *testing.T) context.Context {
				return context.WithValue(t.Context(), ctxKey{}, want)
			},
			check: func(t *testing.T, ok bool, err error) {
				t.Helper()
				if !ok {
					t.Errorf("expected ok=true, got false")
				}
				if err != nil {
					t.Errorf("expected err=nil, got %v", err)
				}
			},
		},
		{
			name: "test-dispatch-canceled",
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx
			},
			check: func(t *testing.T, ok bool, err error) {
				t.Helper()
				if !ok {
					t.Errorf("expected ok=true, got false")
				}
				if !errors.Is(err, context.Canceled) {
					t.Errorf("expected context.Canceled, got %v", err)
				}
			},
		},
		{
			name: "test-dispatch-no-ctx",
			ctx: func(t *testing.T) context.Context {
				return t.Context()
			},
			check: func(t *testing.T, ok bool, err error) {
				t.Helper()
				if !ok {
					t.Errorf("expected ok=true, got false")
				}
				if err != nil {
					t.Errorf("expected err=nil, got %v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			argv0 := os.Args[0]
			t.Cleanup(func() { os.Args[0] = argv0 })

			os.Args[0] = tc.name
			ok, err := reexec.Dispatch(tc.ctx(t))
			tc.check(t, ok, err)
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
		expExit    int
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
		{
			doc:        "entrypoint returns error",
			cmdAndArgs: []string{testReExec},
			expOut:     "entrypoint failed: " + errTestEntrypointFailure.Error(),
			expExit:    1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.doc, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()

			cmd := reexec.CommandContext(ctx, tc.cmdAndArgs...)
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
			switch {
			case tc.expError != nil:
				if !errors.Is(err, tc.expError) {
					t.Errorf("expected %v, got: %v (out: %v)", tc.expError, err, string(out))
				}
			case tc.expExit != 0:
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
				}
				if code := exitErr.ExitCode(); code != tc.expExit {
					t.Errorf("got exit code %d, want %d; out: %v", code, tc.expExit, string(out))
				}
			default:
				if err != nil {
					t.Errorf("unexpected error: %v (out: %v)", err, string(out))
				}
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
// It forces Self() to bypass the Linux /proc/self/exe fast-path via
// [reexecoverride.OverrideArgv0] so that the fallback logic is exercised
// consistently across platforms.
func TestRunNaiveSelf(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	// Force Self() to use naiveSelf(os.Args[0]), instead of "/proc/self/exe" on Linux.
	reexecoverride.OverrideArgv0(t, os.Args[0])

	cmd := reexec.CommandContext(ctx, testReExec2)
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
	t.Run("fast path on Linux", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("only supported on Linux")
		}
		resolved := reexec.Self()
		expected := "/proc/self/exe"
		if resolved != expected {
			t.Errorf("got %v, want %v", resolved, expected)
		}
	})
	t.Run("resolve in PATH", func(t *testing.T) {
		executable := "sh"
		if runtime.GOOS == "windows" {
			executable = "cmd"
		}
		reexecoverride.OverrideArgv0(t, executable)
		resolved := reexec.Self()
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
		reexecoverride.OverrideArgv0(t, executable)
		resolved := reexec.Self()
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
		reexecoverride.OverrideArgv0(t, executable)
		resolved := reexec.Self()
		if resolved != want {
			t.Errorf("expected absolute path; got %q, want %q", resolved, want)
		}
	})
	t.Run("absolute path unchanged", func(t *testing.T) {
		executable := filepath.Join(os.TempDir(), "some-executable")
		reexecoverride.OverrideArgv0(t, executable)
		resolved := reexec.Self()
		if resolved != executable {
			t.Errorf("should not modify absolute paths; got %q, want %q", resolved, executable)
		}
	})
}
