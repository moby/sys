package signal

import (
	"os"
	gosignal "os/signal"
	"sync/atomic"
	"syscall"

	"github.com/Sirupsen/logrus"
)

// Trap sets up a simplified signal "trap", appropriate for common
// behavior expected from a vanilla unix command-line tool in general
// (and the Docker engine in particular).
//
// * If SIGINT or SIGTERM are received, `cleanup` is called, then the process is terminated.
// * If SIGINT or SIGTERM are repeated 3 times before cleanup is complete, then cleanup is
// skipped and the process terminated directly.
// * If "DEBUG" is set in the environment, SIGQUIT causes an exit without cleanup.
//
func Trap(cleanup func()) {
	c := make(chan os.Signal, 1)
	signals := []os.Signal{os.Interrupt, syscall.SIGTERM}
	if os.Getenv("DEBUG") == "" {
		signals = append(signals, syscall.SIGQUIT)
	}
	gosignal.Notify(c, signals...)
	go func() {
		interruptCount := uint32(0)
		for sig := range c {
			go func(sig os.Signal) {
				logrus.Infof("Received signal '%v', starting shutdown of docker...", sig)
				switch sig {
				case os.Interrupt, syscall.SIGTERM:
					// If the user really wants to interrupt, let him do so.
					if atomic.LoadUint32(&interruptCount) < 3 {
						// Initiate the cleanup only once
						if atomic.AddUint32(&interruptCount, 1) == 1 {
							// Call cleanup handler
							cleanup()
							os.Exit(0)
						} else {
							return
						}
					} else {
						logrus.Infof("Force shutdown of docker, interrupting cleanup")
					}
				case syscall.SIGQUIT:
				}
				os.Exit(128 + int(sig.(syscall.Signal)))
			}(sig)
		}
	}()
}
