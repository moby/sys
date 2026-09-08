//go:build !windows && !linux

package user

// lookupUserViaSystemdUserdb is a no-op on non-Linux platforms: systemd
// doesn't exist there, so there's nothing to query.
func lookupUserViaSystemdUserdb(name string) (User, error) {
	return User{}, ErrNoPasswdEntries
}
