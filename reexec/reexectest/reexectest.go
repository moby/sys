// Package reexectest provides small helpers for subprocess tests that re-exec
// the current test binary. The child process is selected by setting argv0 to a
// deterministic token derived from (t.Name(), name), while -test.run is used to
// run only the current test/subtest.
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
//		cmd := reexectest.CommandContext(t, t.Context(), "child", "arg1")
//		out, err := cmd.CombinedOutput()
//		if err != nil {
//			t.Fatalf("child failed: %v\n%s", err, out)
//		}
//	}
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
)

const argv0Prefix = "reexectest-"

// argv0Token returns a short, portable, deterministic argv0 token.
func argv0Token(t *testing.T, name string) string {
	sum := sha256.Sum256([]byte(t.Name() + "\x00" + name))
	// 8 bytes => 16 hex chars; plenty to avoid collisions for test usage.
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

	// Scrub the "-test.run=<pattern>" that was injected by CommandContext
	origArgs := os.Args
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "-test.run=") {
		os.Args = append(os.Args[:1], os.Args[2:]...)
		defer func() { os.Args = origArgs }()
	}

	f(t)
	return true
}

// Command returns an [*exec.Cmd] configured to re-exec the current test binary
// as a subprocess for the given test and name.
//
// The child process is restricted to run only the current test or subtest
// (via -test.run). Its argv[0] is set to a deterministic token derived from
// (t.Name(), name), which is used by [Run] to select the child execution path.
//
// On Linux, the returned command sets [syscall.SysProcAttr.Pdeathsig] to
// SIGTERM, so the child receives SIGTERM if the creating thread dies.
// Callers may modify SysProcAttr before starting the command.
//
// It is analogous to [exec.Command], but targets the current test binary.
func Command(t *testing.T, name string, args ...string) *exec.Cmd {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable(): %v", err)
	}

	argv0 := argv0Token(t, name)
	pattern := "^" + regexp.QuoteMeta(t.Name()) + "$"

	cmd := exec.Command(exe)
	cmd.Path = exe
	cmd.Args = append([]string{argv0, "-test.run=" + pattern}, args...)
	setPdeathsig(cmd)
	return cmd
}

// CommandContext is like [Command] but includes a context. It uses
// [exec.CommandContext] under the hood.
//
// The provided context controls cancellation of the subprocess in the same
// way as [exec.CommandContext].
//
// On Linux, the returned command sets [syscall.SysProcAttr.Pdeathsig] to
// SIGTERM. Callers may modify SysProcAttr before starting the command.
func CommandContext(t *testing.T, ctx context.Context, name string, args ...string) *exec.Cmd {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable(): %v", err)
	}

	argv0 := argv0Token(t, name)
	pattern := "^" + regexp.QuoteMeta(t.Name()) + "$"

	cmd := exec.CommandContext(ctx, exe)
	cmd.Path = exe
	cmd.Args = append([]string{argv0, "-test.run=" + pattern}, args...)
	setPdeathsig(cmd)
	return cmd
}
