//go:build linux

package user

import (
	"bufio"
	"encoding/json"
	"net"
	"time"
)

const userdbMultiplexerSocket = "/run/systemd/userdb/io.systemd.Multiplexer"

// lookupUserViaSystemdUserdb resolves name through systemd-userdbd's
// multiplexer service over Varlink, which consults NSS, systemd-homed, and
// dynamic users in addition to /etc/passwd. It is used as a fallback when
// LookupUser can't find the account by reading /etc/passwd directly.
//
// If the multiplexer socket isn't present (systemd-userdbd not running),
// this returns ErrNoPasswdEntries, same as a normal "not found" result, so
// callers don't need to special-case it.
func lookupUserViaSystemdUserdb(name string) (User, error) {
	return queryUserdb(userdbMultiplexerSocket, name)
}

func queryUserdb(socketPath, name string) (User, error) {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return User{}, ErrNoPasswdEntries
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return User{}, ErrNoPasswdEntries
	}

	req := struct {
		Method     string `json:"method"`
		Parameters struct {
			UserName string `json:"userName"`
			Service  string `json:"service"`
		} `json:"parameters"`
	}{Method: "io.systemd.UserDatabase.GetUserRecord"}
	req.Parameters.UserName = name
	// "service" must name the multiplexer explicitly, or systemd-userdbd
	// rejects the request with a BadService error.
	req.Parameters.Service = "io.systemd.Multiplexer"

	payload, err := json.Marshal(req)
	if err != nil {
		return User{}, err
	}
	// Varlink frames each message with a trailing NUL byte.
	if _, err := conn.Write(append(payload, 0)); err != nil {
		return User{}, ErrNoPasswdEntries
	}

	line, err := bufio.NewReader(conn).ReadBytes(0)
	if err != nil {
		return User{}, ErrNoPasswdEntries
	}

	var reply struct {
		Error      string `json:"error"`
		Parameters struct {
			Record struct {
				UserName string `json:"userName"`
				UID      uint32 `json:"uid"`
			} `json:"record"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(line[:len(line)-1], &reply); err != nil {
		return User{}, ErrNoPasswdEntries
	}
	// A named error (e.g. NoRecordFound) or a record that doesn't actually
	// match the requested name (including an empty one) both mean "no
	// match" -- treat them the same as LookupUser's own not-found case.
	if reply.Error != "" || reply.Parameters.Record.UserName != name {
		return User{}, ErrNoPasswdEntries
	}
	return User{Name: reply.Parameters.Record.UserName, Uid: int(reply.Parameters.Record.UID)}, nil
}
