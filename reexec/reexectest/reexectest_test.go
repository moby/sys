package reexectest_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/moby/sys/reexec/reexectest"
)

func TestRun(t *testing.T) {
	t.Run("env-and-output", func(t *testing.T) {
		const expected = "child-env-and-output-ok"
		if reexectest.Run(t, "env-and-output", func(t *testing.T) {
			if got := os.Getenv("REEXEC_TEST_HELLO"); got != "world" {
				t.Fatalf("env REEXEC_TEST_HELLO: got %q, want %q", got, "world")
			}
			fmt.Println(expected)
		}) {
			return
		}

		cmd := reexectest.CommandContext(t, t.Context(), "env-and-output")
		cmd.Env = append(cmd.Environ(), "REEXEC_TEST_HELLO=world")

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("env-and-output child failed: %v\n%s", err, out)
		}
		if got := strings.TrimSpace(strings.TrimSuffix(string(out), "PASS\n")); got != expected {
			t.Errorf("env-and-output output: got %q, want %q", got, expected)
		}
	})

	t.Run("exit-code", func(t *testing.T) {
		if reexectest.Run(t, "exit-code", func(t *testing.T) {
			os.Exit(23)
		}) {
			return
		}

		cmd := reexectest.CommandContext(t, t.Context(), "exit-code")
		err := cmd.Run()
		if err == nil {
			t.Fatalf("expected non-nil error")
		}

		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("got %T, want *exec.ExitError", err)
		}
		if code := ee.ProcessState.ExitCode(); code != 23 {
			t.Fatalf("exit code: got %d, want %d", code, 23)
		}
	})

	t.Run("args-passthrough", func(t *testing.T) {
		const expected = "child-args-passthrough-ok"
		if reexectest.Run(t, "args-passthrough", func(t *testing.T) {
			want := []string{"hello", "world"}
			got := os.Args[1:]
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("args: got %q, want %q (full os.Args=%q)", got, want, os.Args)
			}
			fmt.Println(expected)
		}) {
			return
		}

		cmd := reexectest.CommandContext(t, t.Context(), "args-passthrough", "hello", "world")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("args-passthrough child failed: %v\n%s", err, out)
		}
		if got := strings.TrimSpace(strings.TrimSuffix(string(out), "PASS\n")); got != expected {
			t.Errorf("args-passthrough output: got %q, want %q", got, expected)
		}
	})
}

func TestRunNonSubtest(t *testing.T) {
	const expected = "child-non-sub-test-ok"
	if reexectest.Run(t, "non-sub-test", func(t *testing.T) {
		fmt.Println(expected)
	}) {
		return
	}

	cmd := reexectest.CommandContext(t, t.Context(), "non-sub-test")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("non-sub-test child failed: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(strings.TrimSuffix(string(out), "PASS\n")); got != expected {
		t.Errorf("non-sub-test output: got %q, want %q", got, expected)
	}
}
