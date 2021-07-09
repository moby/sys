// +build linux

package signal

import (
	"io/ioutil"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

func buildTestBinary(t *testing.T, tmpdir string, prefix string) (string, string) {
	t.Helper()
	tmpDir, err := ioutil.TempDir(tmpdir, prefix)
	if err != nil {
		t.Fatal(err)
	}
	exePath := tmpDir + "/" + prefix
	wd, _ := os.Getwd()
	testHelperCode := wd + "/testfiles/main.go"
	cmd := exec.Command("go", "build", "-o", exePath, testHelperCode)
	err = cmd.Run()
	if err != nil {
		t.Fatal(err)
	}
	return exePath, tmpDir
}

func TestTrap(t *testing.T) {
	var sigmap = []struct {
		name     string
		signal   os.Signal
		multiple bool
	}{
		{"TERM", syscall.SIGTERM, false},
		{"QUIT", syscall.SIGQUIT, true},
		{"INT", os.Interrupt, false},
		{"TERM", syscall.SIGTERM, true},
		{"INT", os.Interrupt, true},
	}
	exePath, tmpDir := buildTestBinary(t, "", "main")
	defer os.RemoveAll(tmpDir)

	for _, v := range sigmap {
		t.Run(v.name, func(t *testing.T) {
			cmd := exec.Command(exePath)
			cmd.Env = append(os.Environ(), "SIGNAL_TYPE="+v.name)
			if v.multiple {
				cmd.Env = append(cmd.Env, "IF_MULTIPLE=1")
			}
			err := cmd.Start()
			if err != nil {
				t.Fatal(err)
			}
			err = cmd.Wait()
			e, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("expected exec.ExitError, got %T", e)
			}

			code := e.Sys().(syscall.WaitStatus).ExitStatus()
			if v.multiple {
				if !reflect.DeepEqual(128+int(v.signal.(syscall.Signal)), code) {
					t.Fatalf("expected: %v, got: %v", 128+int(v.signal.(syscall.Signal)), code)
				}
			} else {
				if code != 99 {
					t.Fatalf("expected: %v, got: %v", 99, code)
				}
			}
		})
	}

}

func TestDumpStacks(t *testing.T) {
	directory, err := ioutil.TempDir("", "test-dump-tasks")
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(directory)
	dumpPath, err := DumpStacks(directory)
	if err != nil {
		t.Error(err)
	}
	readFile, _ := ioutil.ReadFile(dumpPath)
	fileData := string(readFile)
	if !strings.Contains(fileData, "goroutine") {
		t.Error("dump does not contain 'goroutine'")
	}
}

func TestDumpStacksWithEmptyInput(t *testing.T) {
	path, err := DumpStacks("")
	if err != nil {
		t.Error(err)
	}
	expected := os.Stderr.Name()
	if path != os.Stderr.Name() {
		t.Fatalf("expected: %v, got: %v", expected, path)
	}
}
