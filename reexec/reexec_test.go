package reexec

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	testReExec  = "test-reexec"
	testReExec2 = "test-reexec2"
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
