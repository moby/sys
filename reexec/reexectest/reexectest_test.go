package reexectest_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/moby/sys/reexec/reexectest"
)

// assertOutput verifies the output produced by a reexec command. The test
// binary may append harness output after PASS, for example when coverage is
// enabled, so only output before the PASS line is compared.
func assertOutput(t *testing.T, out []byte, expected string) {
	t.Helper()

	got := string(out)
	if before, _, ok := strings.Cut(got, "\nPASS\n"); ok {
		got = before
	}
	got = strings.TrimSpace(got)

	if got != expected {
		t.Errorf("output: got %q, want %q\nfull output:\n%s", got, expected, out)
	}
}

// TestAssertOutput verifies handling of test-harness output appended by the
// reexecuted test binary.
func TestAssertOutput(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "plain",
			out: `child output
PASS
`,
			want: "child output",
		},
		{
			name: "coverage",
			out: `child output
PASS
coverage: 31.7% of statements
`,
			want: "child output",
		},
		{
			name: "multiline",
			out: `first line of child output
second line of child output
PASS
`,
			want: `first line of child output
second line of child output`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertOutput(t, []byte(tc.out), tc.want)
		})
	}
}

// TestRun verifies the basic reexec behavior, including environment, exit
// status, argument passing, and context handling.
func TestRun(t *testing.T) {
	// Verify that the child inherits the configured environment and writes output.
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

		cmd := reexectest.Command(t, "env-and-output")
		cmd.Env = append(cmd.Environ(), "REEXEC_TEST_HELLO=world")

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("command failed: %v\n%s", err, out)
		}
		assertOutput(t, out, expected)
	})

	// Verify that the child process exit status is propagated to the parent.
	t.Run("exit-code", func(t *testing.T) {
		if reexectest.Run(t, "exit-code", func(t *testing.T) {
			os.Exit(23)
		}) {
			return
		}

		cmd := reexectest.Command(t, "exit-code")
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

	// Verify that child arguments, including flag-like arguments, are passed through unchanged.
	t.Run("args-passthrough", func(t *testing.T) {
		const expected = "child-args-passthrough-ok"
		if reexectest.Run(t, "args-passthrough", func(t *testing.T) {
			want := []string{"hello", "-flag", "-test.run=bogus", "world"}
			got := os.Args[1:]
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("args: got %q, want %q (full os.Args=%q)", got, want, os.Args)
			}
			fmt.Println(expected)
		}) {
			return
		}

		cmd := reexectest.Command(t, "args-passthrough", "hello", "-flag", "-test.run=bogus", "world")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("command failed: %v\n%s", err, out)
		}
		assertOutput(t, out, expected)
	})

	// Verify that flag-like child arguments can be parsed independently of testing flags.
	t.Run("custom-flags", func(t *testing.T) {
		const expected = "child-custom-flags-ok"
		if reexectest.Run(t, "custom-flags", func(t *testing.T) {
			flags := flag.NewFlagSet("child", flag.ContinueOnError)
			value := flags.String("custom", "", "")
			if err := flags.Parse(os.Args[1:]); err != nil {
				t.Fatal(err)
			}
			if *value != "hello" {
				t.Fatalf("custom flag: got %q, want %q", *value, "hello")
			}
			fmt.Println(expected)
		}) {
			return
		}

		cmd := reexectest.Command(t, "custom-flags", "-custom=hello")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("command failed: %v\n%s", err, out)
		}
		assertOutput(t, out, expected)
	})

	// Verify that CommandContext reexecs the child with the provided context.
	t.Run("context", func(t *testing.T) {
		const expected = "child-context-ok"
		if reexectest.Run(t, "context", func(t *testing.T) {
			fmt.Println(expected)
		}) {
			return
		}

		cmd := reexectest.CommandContext(t, t.Context(), "context")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("command failed: %v\n%s", err, out)
		}
		assertOutput(t, out, expected)
	})

	// Verify that CommandContext honors cancellation before starting the child.
	t.Run("context-cancel", func(t *testing.T) {
		if reexectest.Run(t, "context-cancel", func(t *testing.T) {
			t.Fatal("unexpected child execution")
		}) {
			return
		}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		cmd := reexectest.CommandContext(t, ctx, "context-cancel")
		err := cmd.Run()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	})

	// Verify that name selects one of multiple reexec handlers in the same test.
	t.Run("named-handlers", func(t *testing.T) {
		const expected = "second-handler-ok"

		if reexectest.Run(t, "first", func(t *testing.T) {
			t.Fatal("unexpected first handler")
		}) {
			return
		}
		if reexectest.Run(t, "second", func(t *testing.T) {
			fmt.Println(expected)
		}) {
			return
		}

		cmd := reexectest.Command(t, "second")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("command failed: %v\n%s", err, out)
		}
		assertOutput(t, out, expected)
	})

	// Verify that Run keeps the injected test arguments scrubbed until child
	// cleanup callbacks have completed.
	t.Run("cleanup-args", func(t *testing.T) {
		if reexectest.Run(t, "cleanup-args", func(t *testing.T) {
			want := []string{"hello"}

			t.Cleanup(func() {
				if got := os.Args[1:]; !reflect.DeepEqual(got, want) {
					t.Errorf("cleanup args: got %q, want %q", got, want)
				}
			})
		}) {
			return
		}

		cmd := reexectest.Command(t, "cleanup-args", "hello")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("command failed: %v\n%s", err, out)
		}
	})
}

// TestRunTopLevel verifies that reexec works when used directly from a
// top-level test.
func TestRunTopLevel(t *testing.T) {
	const expected = "child-non-sub-test-ok"
	if reexectest.Run(t, "non-sub-test", func(t *testing.T) {
		fmt.Println(expected)
	}) {
		return
	}

	cmd := reexectest.Command(t, "non-sub-test")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("command failed: %v\n%s", err, out)
	}
	assertOutput(t, out, expected)
}

// runPatternSubtestName is shared by TestRunPattern and its canary test so an
// over-broad match of the top-level test name would otherwise select both.
const runPatternSubtestName = "child"

// TestRunPattern verifies that each component of the generated -test.run
// pattern is matched exactly. TestRunPatternExtra is the corresponding canary
// that detects if this reexec child also selects a prefix-matching test.
func TestRunPattern(t *testing.T) {
	t.Run(runPatternSubtestName, func(t *testing.T) {
		const expected = "child-ok"
		if reexectest.Run(t, runPatternSubtestName, func(t *testing.T) {
			fmt.Println(expected)
		}) {
			return
		}

		cmd := reexectest.Command(t, runPatternSubtestName)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("command failed: %v\n%s", err, out)
		}
		assertOutput(t, out, expected)
	})
}

// TestRunPatternExtra is the canary for TestRunPattern. It intentionally has
// the same subtest name and a prefix-matching top-level test name, and must not
// be selected by TestRunPattern's reexec child.
func TestRunPatternExtra(t *testing.T) {
	t.Run(runPatternSubtestName, func(t *testing.T) {
		if len(os.Args) >= 3 && os.Args[2] == "--" {
			t.Fatal("unexpected selection by a reexec child")
		}
	})
}

// TestRunPatternSpecialNames verifies that test names containing regexp
// metacharacters and separators are handled correctly when constructing
// -test.run patterns.
func TestRunPatternSpecialNames(t *testing.T) {
	for _, name := range []string{
		"regexp.*chars",
		"slash/name",
	} {
		t.Run(name, func(t *testing.T) {
			// Verify that the exact generated test name can be selected even when it
			// contains characters that are significant to -test.run matching.
			const expected = "child-ok"
			if reexectest.Run(t, "child", func(t *testing.T) {
				fmt.Println(expected)
			}) {
				return
			}

			cmd := reexectest.Command(t, "child")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("command failed: %v\n%s", err, out)
			}
			assertOutput(t, out, expected)
		})
	}
}
