package reexec_test

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/moby/sys/reexec"
)

func init() {
	reexec.Register("example-child", func() {
		fmt.Println("Hello from example-child entrypoint")
	})
}

// Example_programmatic demonstrates using reexec to programmatically
// re-execute the current binary.
func Example_programmatic() {
	if reexec.Init() {
		// Matched a reexec entrypoint; stop normal main execution.
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
