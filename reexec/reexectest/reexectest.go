// Package reexectest provides helpers for subprocess tests that re-exec the
// current test binary. The child process is selected by setting argv0 to a
// deterministic token derived from (t.Name(), name), while -test.run is used
// to run only the current test or subtest.
//
// Typical usage:
//
//	func TestSomething(t *testing.T) {
//		if reexectest.Run(t, "child", func(t *testing.T) {
//			// child branch
//		}) {
//			return
//		}
//
//		// parent branch
//		cmd := reexectest.Command(t, "child", "arg1")
//		out, err := cmd.CombinedOutput()
//		if err != nil {
//			t.Fatalf("child failed: %v\n%s", err, out)
//		}
//	}
//
// Arguments passed to [Command] or [CommandContext] are forwarded to the child
// unchanged. Arguments beginning with "-" are not interpreted as flags by the
// test binary and may be parsed by the child as needed.
package reexectest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/moby/sys/reexec"
)

const argv0Prefix = "reexectest-"

// argv0Token returns a short (16 hex chars) deterministic argv0 token
// based on the test's name to prevent collisions.
func argv0Token(t *testing.T, name string) string {
	sum := sha256.Sum256([]byte(t.Name() + "\x00" + name))
	return argv0Prefix + hex.EncodeToString(sum[:8])
}

// Run runs f in the current process iff it is the matching child process for
// (t, name). It returns true if f ran (i.e., we are the child).
//
// When Run returns true, callers should return from the test to avoid running
// the parent branch in the child process.
func Run(t *testing.T, name string, f func(t *testing.T)) bool {
	t.Helper()

	if os.Args[0] != argv0Token(t, name) {
		return false
	}

	// Validate the arguments injected by CommandContext. Arguments after "--"
	// belong to the caller and are passed through unchanged.
	if len(os.Args) < 3 ||
		!strings.HasPrefix(os.Args[1], "-test.run=") ||
		os.Args[2] != "--" {
		t.Fatalf("unexpected reexec arguments: %q", os.Args)
	}

	// Scrub the injected test arguments for the lifetime of the child test.
	origArgs := os.Args
	os.Args = append([]string{os.Args[0]}, os.Args[3:]...)
	t.Cleanup(func() {
		os.Args = origArgs
	})

	f(t)
	return true
}

// Command returns an [*exec.Cmd] configured to re-exec the current test binary
// as a subprocess for the given test and name.
//
// It is a convenience wrapper around [CommandContext] using [testing.T.Context]
// as context.
func Command(t *testing.T, name string, args ...string) *exec.Cmd {
	return commandContext(t, t.Context(), name, args...)
}

// CommandContext returns an [*exec.Cmd] configured to re-exec the current test
// binary as a subprocess for the given test and name.
//
// The child process is restricted to run only the current test or subtest
// via -test.run. Its argv[0] is set to a deterministic token derived from
// (t.Name(), name), which is used by [Run] to select the child execution path.
//
// The provided context controls cancellation of the subprocess in the same way
// as [exec.CommandContext].
//
// On Linux, the returned command has [syscall.SysProcAttr.Pdeathsig] set to
// SIGTERM, so the child receives SIGTERM if the creating thread dies. Callers
// may modify SysProcAttr before starting the command.
//
// It is analogous to [exec.CommandContext], but targets the current test binary.
func CommandContext(t *testing.T, ctx context.Context, name string, args ...string) *exec.Cmd {
	return commandContext(t, ctx, name, args...)
}

func commandContext(t *testing.T, ctx context.Context, name string, args ...string) *exec.Cmd {
	argv0 := argv0Token(t, name)
	pattern := testRunPattern(t.Name())

	cmd := reexec.CommandContext(ctx, argv0, "-test.run="+pattern, "--")
	cmd.Args = append(cmd.Args, args...)
	return cmd
}

func testRunPattern(name string) string {
	parts := strings.Split(name, "/")
	for i := range parts {
		parts[i] = "^" + regexp.QuoteMeta(parts[i]) + "$"
	}
	return strings.Join(parts, "/")
}
