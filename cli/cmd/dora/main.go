package main

import (
	"fmt"
	"os"

	command "github.com/MehulxBuilds/dora/cli/cmd"
)

func main() {
	if err := command.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dora:", err)
		os.Exit(1)
	}
}
