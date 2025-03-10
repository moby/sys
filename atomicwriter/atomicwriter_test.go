package atomicwriter

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testMode returns the file-mode to use in tests, accounting for Windows
// not supporting full Linux file mode.
func testMode() os.FileMode {
	if runtime.GOOS == "windows" {
		return 0o666
	}
	return 0o640
}

// assertFile asserts the given fileName to exist, and to have the expected
// content and mode.
func assertFile(t *testing.T, fileName string, fileContent []byte, expectedMode os.FileMode) {
	t.Helper()
	actual, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("Error reading from file: %v", err)
	}

	if !bytes.Equal(actual, fileContent) {
		t.Errorf("Data mismatch, expected %q, got %q", fileContent, actual)
	}

	st, err := os.Stat(fileName)
	if err != nil {
		t.Fatalf("Error statting file: %v", err)
	}
	if st.Mode() != expectedMode {
		t.Errorf("Mode mismatched, expected %o, got %o", expectedMode, st.Mode())
	}
}

// assertFileCount asserts the given directory has the expected number
// of files, and returns the list of files found.
func assertFileCount(t *testing.T, directory string, expected int) []os.DirEntry {
	t.Helper()
	files, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("Error reading dir: %v", err)
	}
	if len(files) != expected {
		t.Errorf("Expected %d files, got %d: %v", expected, len(files), files)
	}
	return files
}

func TestNew(t *testing.T) {
	for _, tc := range []string{"normal", "symlinked"} {
		tmpDir := t.TempDir()
		parentDir := tmpDir
		actualParentDir := parentDir
		if tc == "symlinked" {
			actualParentDir = filepath.Join(tmpDir, "parent-dir")
			if err := os.Mkdir(actualParentDir, 0o700); err != nil {
				t.Fatal(err)
			}
			parentDir = filepath.Join(tmpDir, "parent-dir-symlink")
			if err := os.Symlink(actualParentDir, parentDir); err != nil {
				t.Fatal(err)
			}
		}
		t.Run(tc, func(t *testing.T) {
			for _, tc := range []string{"new-file", "existing-file"} {
				t.Run(tc, func(t *testing.T) {
					fileName := filepath.Join(parentDir, "test.txt")
					var origFileCount int
					if tc == "existing-file" {
						if err := os.WriteFile(fileName, []byte("original content"), testMode()); err != nil {
							t.Fatalf("Error writing file: %v", err)
						}
						origFileCount = 1
					}
					writer, err := New(fileName, testMode())
					if writer == nil {
						t.Errorf("Writer is nil")
					}
					if err != nil {
						t.Fatalf("Error creating new atomicwriter: %v", err)
					}
					files := assertFileCount(t, actualParentDir, origFileCount+1)
					if tmpFileName := files[0].Name(); !strings.HasPrefix(tmpFileName, ".tmp-test.txt") {
						t.Errorf("Unexpected file name for temp-file: %s", tmpFileName)
					}

					// Closing the writer without writing should clean up the temp-file,
					// and should not replace the destination file.
					if err = writer.Close(); err != nil {
						t.Errorf("Error closing writer: %v", err)
					}
					assertFileCount(t, actualParentDir, origFileCount)
					if tc == "existing-file" {
						assertFile(t, fileName, []byte("original content"), testMode())
					}
				})
			}
		})
	}
}

func TestNewInvalid(t *testing.T) {
	t.Run("missing target dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		fileName := filepath.Join(tmpDir, "missing-dir", "test.txt")
		writer, err := New(fileName, testMode())
		if writer != nil {
			t.Errorf("Should not have created writer")
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Should produce a 'not found' error, but got %[1]T (%[1]v)", err)
		}
	})
}

func TestWriteFile(t *testing.T) {
	tmpDir := t.TempDir()

	fileName := filepath.Join(tmpDir, "test.txt")
	fileContent := []byte("file content")
	fileMode := testMode()
	if err := WriteFile(fileName, fileContent, fileMode); err != nil {
		t.Fatalf("Error writing to file: %v", err)
	}
	assertFile(t, fileName, fileContent, fileMode)
}

func TestWriteSetCommit(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.Mkdir(filepath.Join(tmpDir, "tmp"), 0o700); err != nil {
		t.Fatalf("Error creating tmp directory: %s", err)
	}

	targetDir := filepath.Join(tmpDir, "target")
	ws, err := NewWriteSet(filepath.Join(tmpDir, "tmp"))
	if err != nil {
		t.Fatalf("Error creating atomic write set: %s", err)
	}

	fileContent := []byte("file content")
	fileMode := testMode()

	if err := ws.WriteFile("foo", fileContent, fileMode); err != nil {
		t.Fatalf("Error writing to file: %v", err)
	}

	if _, err := os.ReadFile(filepath.Join(targetDir, "foo")); err == nil {
		t.Fatalf("Expected error reading file where should not exist")
	}

	if err := ws.Commit(targetDir); err != nil {
		t.Fatalf("Error committing file: %s", err)
	}

	assertFile(t, filepath.Join(targetDir, "foo"), fileContent, fileMode)
}

func TestWriteSetCancel(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.Mkdir(filepath.Join(tmpDir, "tmp"), 0o700); err != nil {
		t.Fatalf("Error creating tmp directory: %s", err)
	}

	ws, err := NewWriteSet(filepath.Join(tmpDir, "tmp"))
	if err != nil {
		t.Fatalf("Error creating atomic write set: %s", err)
	}

	fileContent := []byte("file content")
	fileMode := testMode()
	if err := ws.WriteFile("foo", fileContent, fileMode); err != nil {
		t.Fatalf("Error writing to file: %v", err)
	}

	if err := ws.Cancel(); err != nil {
		t.Fatalf("Error committing file: %s", err)
	}

	if _, err := os.ReadFile(filepath.Join(tmpDir, "target", "foo")); err == nil {
		t.Fatalf("Expected error reading file where should not exist")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Unexpected error reading file: %s", err)
	}
}
