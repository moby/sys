// +build go1.13

package mount

// Mount will mount filesystem according to the specified configuration.
// Options must be specified like the mount or fstab unix commands:
// "opt1=val1,opt2=val2". See flags.go for supported option flags.
//
// Mount is not implemented on Windows.
func Mount(device, target, mType, options string) error {
	flag, data := parseOptions(options)
	return mount(device, target, mType, uintptr(flag), data)
}

// Unmount lazily unmounts a filesystem on supported platforms, otherwise does
// a normal unmount. If target is not a mount point, no error is returned.
//
// Unmount is not implemented on Windows.
func Unmount(target string) error {
	return unmount(target, mntDetach)
}

// RecursiveUnmount unmounts the target and all mounts underneath, starting
// with the deepest mount first. The argument does not have to be a mount
// point itself.
//
// RecursiveUnmount is not implemented on Windows.
func RecursiveUnmount(target string) error {
	return recursiveUnmount(target)
}
