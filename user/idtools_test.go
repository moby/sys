package user_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/moby/sys/user"
)

// TestMkdirAndChownNonDir checks that MkdirAndChown returns a correct error in case
// a directory which it is about to create already exists but is a file (rather
// than a directory).
func TestMkdirAndChownNonDir(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), t.Name())
	if err != nil {
		t.Fatalf("Couldn't create temp dir: %v", err)
	}
	_ = file.Close()

	expected := syscall.ENOTDIR
	err = user.MkdirAndChown(file.Name(), 0o755, 0, 0)
	if !errors.Is(err, expected) {
		t.Fatalf("expected error: %v, got: %v", expected, err)
	}
}

// TestMkdirAndChownExistingDir checks that MkdirAndChown does not return an error
// if the target directory already exists.
func TestMkdirAndChownExistingDir(t *testing.T) {
	dirName := t.TempDir()
	err := user.MkdirAndChown(dirName, 0, 0, 0, user.WithOnlyNew)
	if err != nil {
		t.Fatal(err)
	}
}

// TestMkdirAndChownMissingParent checks that MkdirAndChown errors if the parent
// directory doesn't exist and doesn't create any of the parent directories.
func TestMkdirAndChownMissingParent(t *testing.T) {
	dirName := t.TempDir()
	if err := user.MkdirAndChown(filepath.Join(dirName, "usr", "bin", "subdir"), 0, 0, 0, user.WithOnlyNew); err == nil {
		t.Fatal("Trying to create a directory with Mkdir where the parent doesn't exist should have failed")
	}

	_, err := os.Stat(filepath.Join(dirName, "usr"))
	if err == nil || !os.IsNotExist(err) {
		t.Fatal("parent directory should not have been created", err)
	}
	_, err = os.Stat(filepath.Join(dirName, "usr", "bin"))
	if err == nil || !os.IsNotExist(err) {
		t.Fatal("parent directory should not have been created", err)
	}
	_, err = os.Stat(filepath.Join(dirName, "usr", "bin", "subdir"))
	if err == nil || !os.IsNotExist(err) {
		t.Fatal("directory should not have been created", err)
	}
}
