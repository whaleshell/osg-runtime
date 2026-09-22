package main

import (
	"fmt"
	"os"

	"github.com/whaleshell/whaleshell-runtime/app/agent"
)

func main() {
	if err := agent.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
