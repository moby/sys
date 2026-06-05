package reexec_test

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/moby/sys/reexec"
)

func init() {
	reexec.RegisterContext("example-child", func(context.Context) error {
		fmt.Println("Hello from example-child entrypoint")
		return nil
	})
}

// Example_programmatic demonstrates using reexec to programmatically
// re-execute the current binary.
func Example_programmatic() {
	ctx := context.Background()
	if ok, err := reexec.Dispatch(ctx); ok {
		if err != nil {
			fmt.Fprintln(os.Stderr, "entrypoint failed:", err)
			os.Exit(1)
		}
		// Matched a reexec entrypoint; stop normal execution.
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := reexec.CommandContext(ctx, "example-child")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Println("reexec error:", err)
		return
	}

	fmt.Println("Back in parent process")
	// Output:
	// Hello from example-child entrypoint
	// Back in parent process
}
