// Package signal provides helper functions for dealing with signals across
// various operating systems.
package signal

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

// CatchAll catches all signals and relays them to the specified channel.
// SIGURG is ignored, as it is used by the Go runtime for internal purposes
// (see https://github.com/golang/go/issues/24543).
func CatchAll(sigc chan os.Signal) {
	var handledSigs []os.Signal
	for n, s := range SignalMap {
		if n == "URG" {
			continue
		}
		handledSigs = append(handledSigs, s)
	}
	signal.Notify(sigc, handledSigs...)
}

// StopCatch stops catching the signals and closes the specified channel.
func StopCatch(sigc chan os.Signal) {
	signal.Stop(sigc)
	close(sigc)
}

// ParseSignal translates a string to a valid syscall signal.
// It returns an error if the signal map doesn't include the given signal.
func ParseSignal(rawSignal string) (syscall.Signal, error) {
	s, err := strconv.Atoi(rawSignal)
	if err == nil {
		if s == 0 {
			return -1, fmt.Errorf("invalid signal: %s", rawSignal)
		}
		return syscall.Signal(s), nil
	}
	signal, ok := SignalMap[strings.TrimPrefix(strings.ToUpper(rawSignal), "SIG")]
	if !ok {
		return -1, fmt.Errorf("invalid signal: %s", rawSignal)
	}
	return signal, nil
}

// ValidSignalForPlatform returns true if a signal is valid on the platform
func ValidSignalForPlatform(sig syscall.Signal) bool {
	for _, v := range SignalMap {
		if v == sig {
			return true
		}
	}
	return false
}
