package atomicwriter

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fileRenameInformation is the FILE_RENAME_INFORMATION structure used by
// NtSetInformationFile to rename a file. FileName is a variable-length
// field; callers must allocate a buffer large enough to hold the full name.
type fileRenameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

// fileRenameInformationEx is the FILE_RENAME_INFORMATION_EX structure used by
// NtSetInformationFile to rename a file.
type fileRenameInformationEx struct {
	Flags          uint32
	RootDirectory  windows.Handle
	FileNameLength uint32
	FileName       [1]uint16
}

// atomicwriterRename renames oldpath to newpath using os.Rename. If it fails,
// it attempts to use NtSetInformationFile with FILE_RENAME_POSIX_SEMANTICS,
// which allows atomic replacement of a file even when the destination
// is open by another process.
func atomicwriterRename(oldpath, newpath string) error {

	err := os.Rename(oldpath, newpath)
	if err == nil {
		return nil
	}
	// Open the source file requesting DELETE access so we can rename it.
	srcPtr, err := windows.UTF16PtrFromString(oldpath)
	if err != nil {
		return &os.PathError{Op: "rename", Path: oldpath, Err: err}
	}
	handle, err := windows.CreateFile(
		srcPtr,
		windows.DELETE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return &os.PathError{Op: "rename", Path: oldpath, Err: err}
	}
	defer windows.CloseHandle(handle)

	// NtSetInformationFile requires an absolute NT path (\??\C:\...) when
	// RootDirectory is NULL.
	ntNewPath := `\??\` + newpath
	newPathUTF16, err := windows.UTF16FromString(ntNewPath)
	if err != nil {
		return &os.PathError{Op: "rename", Path: newpath, Err: err}
	}

	fileNameLen := len(newPathUTF16)*2 - 2 // byte length, excluding null terminator
	renameInfoEx := fileRenameInformationEx{
		Flags: windows.FILE_RENAME_REPLACE_IF_EXISTS |
			windows.FILE_RENAME_POSIX_SEMANTICS,
	}
	var dummyEx fileRenameInformationEx
	bufferSizeEx := int(unsafe.Offsetof(dummyEx.FileName)) + fileNameLen
	bufferEx := make([]byte, bufferSizeEx)
	infoEx := (*fileRenameInformationEx)(unsafe.Pointer(&bufferEx[0]))
	infoEx.Flags = renameInfoEx.Flags
	infoEx.FileNameLength = uint32(fileNameLen)
	copy((*[windows.MAX_LONG_PATH]uint16)(unsafe.Pointer(&infoEx.FileName[0]))[:fileNameLen/2:fileNameLen/2], newPathUTF16)

	const (
		FileRenameInformation   = 10
		FileRenameInformationEx = 65
	)
	var iosbEx windows.IO_STATUS_BLOCK

	err = windows.NtSetInformationFile(handle, &iosbEx, &bufferEx[0], uint32(bufferSizeEx), FileRenameInformationEx)
	if err == nil {
		return nil
	}

	// If the extended rename fails, fall back to the original FILE_RENAME_INFORMATION
	// which is supported on older versions of Windows. This may fail if the destination
	// file is open by another process, but there's no way to detect that beforehand.

	var dummy fileRenameInformation
	bufferSize := int(unsafe.Offsetof(dummy.FileName)) + fileNameLen
	buffer := make([]byte, bufferSize)
	info := (*fileRenameInformation)(unsafe.Pointer(&buffer[0]))
	info.ReplaceIfExists = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	info.FileNameLength = uint32(fileNameLen)
	copy((*[windows.MAX_LONG_PATH]uint16)(unsafe.Pointer(&info.FileName[0]))[:fileNameLen/2:fileNameLen/2], newPathUTF16)

	var iosb windows.IO_STATUS_BLOCK
	if err := windows.NtSetInformationFile(handle, &iosb, &buffer[0], uint32(bufferSize), FileRenameInformation); err != nil {
		return &os.PathError{Op: "rename", Path: newpath, Err: err}
	}
	return nil
}
