package reexec_test

import (
	"fmt"
	"os"

	"github.com/moby/sys/reexec"
)

func init() {
	reexec.Register("example-foo", func() {
		fmt.Println("Hello from entrypoint example-foo")
	})
	reexec.Register("example-bar", func() {
		fmt.Println("Hello from entrypoint example-bar")
	})
}

// Example_multicall demonstrates a BusyBox-style multi-call binary.
//
// In a real multi-call binary:
//
//	go build -o example .
//	ln -s example example-foo
//	ln -s example example-bar
//
//	./example-foo  # runs entrypoint "example-foo"
//	./example-bar  # runs entrypoint "example-bar"
//
// At process startup, main would call [reexec.Init] and return if it
// matches an entrypoint. This example first shows that call, then emulates
// different invocation names by modifying os.Args[0].
func Example_multicall() {
	// What main would normally do:
	if reexec.Init() {
		// Matched a reexec entrypoint; stop normal main execution.
		return
	}
	reset := os.Args[0]

	// Emulate running as "example-foo".
	os.Args[0] = "example-foo"
	_ = reexec.Init()

	// Emulate running as "example-bar".
	os.Args[0] = "example-bar"
	_ = reexec.Init()

	// Emulate running under the default binary name (no match).
	os.Args[0] = reset
	if !reexec.Init() {
		fmt.Println("Hello main")
	}

	// Output:
	// Hello from entrypoint example-foo
	// Hello from entrypoint example-bar
	// Hello main
}
