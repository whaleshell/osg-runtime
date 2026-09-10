// Command osg-init runs inside the sandbox container: harden, then exec the agent.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "osg-init: stub — harden path lands in P4")
	os.Exit(1)
}
