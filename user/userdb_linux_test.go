//go:build linux

package user

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
)

func TestQueryUserdb(t *testing.T) {
	tests := []struct {
		name    string
		reply   string // raw Varlink reply the fake server sends back
		wantErr bool
		wantUID int
	}{
		{
			name:    "found",
			reply:   `{"parameters":{"record":{"userName":"alice","uid":1000}}}`,
			wantUID: 1000,
		},
		{
			name:    "no record found error",
			reply:   `{"error":"io.systemd.UserDatabase.NoRecordFound","parameters":{}}`,
			wantErr: true,
		},
		{
			// Regression guard: an earlier attempt at this in the Go
			// standard library (CL 459455) was reverted for treating an
			// empty record as a successful match instead of "not found".
			name:    "empty record is not a match",
			reply:   `{"parameters":{"record":{}}}`,
			wantErr: true,
		},
		{
			name:    "malformed reply",
			reply:   `not json`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sock := filepath.Join(t.TempDir(), "userdb.sock")
			ln, err := net.Listen("unix", sock)
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			defer ln.Close()

			go func() {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				defer conn.Close()

				req, err := bufio.NewReader(conn).ReadBytes(0)
				if err != nil {
					return
				}
				var parsed struct {
					Method     string `json:"method"`
					Parameters struct {
						UserName string `json:"userName"`
						Service  string `json:"service"`
					} `json:"parameters"`
				}
				if err := json.Unmarshal(req[:len(req)-1], &parsed); err != nil {
					t.Errorf("server: bad request JSON: %v", err)
					return
				}
				if parsed.Method != "io.systemd.UserDatabase.GetUserRecord" {
					t.Errorf("server: method = %q, want io.systemd.UserDatabase.GetUserRecord", parsed.Method)
				}
				if parsed.Parameters.Service != "io.systemd.Multiplexer" {
					t.Errorf("server: service = %q, want io.systemd.Multiplexer", parsed.Parameters.Service)
				}
				conn.Write(append([]byte(tc.reply), 0))
			}()

			got, err := queryUserdb(sock, "alice")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("queryUserdb() = %+v, nil, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("queryUserdb(): %v", err)
			}
			if got.Name != "alice" || got.Uid != tc.wantUID {
				t.Fatalf("queryUserdb() = %+v, want Name=alice Uid=%d", got, tc.wantUID)
			}
		})
	}

	t.Run("socket missing", func(t *testing.T) {
		sock := filepath.Join(t.TempDir(), "does-not-exist.sock")
		if _, err := queryUserdb(sock, "alice"); err == nil {
			t.Fatal("queryUserdb() with no listener = nil error, want ErrNoPasswdEntries")
		}
	})
}
