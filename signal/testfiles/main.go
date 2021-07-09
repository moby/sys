package main

import (
	"log"
	"os"
	"syscall"
	"time"

	"github.com/moby/sys/signal"
)

var std = log.New(os.Stderr, "", log.LstdFlags)

type logger struct{}

func (l logger) Info(v ...interface{}) {
	// use log.Default() once we no longer support Go 1.15 and older
	std.Print(v...)
}

func main() {
	sigmap := map[string]os.Signal{
		"TERM": syscall.SIGTERM,
		"QUIT": syscall.SIGQUIT,
		"INT":  os.Interrupt,
	}
	signal.Trap(func() {
		time.Sleep(time.Second)
		os.Exit(99)
	}, logger{})
	go func() {
		p, err := os.FindProcess(os.Getpid())
		if err != nil {
			panic(err)
		}
		s := os.Getenv("SIGNAL_TYPE")
		multiple := os.Getenv("IF_MULTIPLE")
		switch s {
		case "TERM", "INT":
			if multiple == "1" {
				for {
					p.Signal(sigmap[s])
				}
			} else {
				p.Signal(sigmap[s])
			}
		case "QUIT":
			p.Signal(sigmap[s])
		}
	}()
	time.Sleep(2 * time.Second)
}
