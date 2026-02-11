package main

import (
	"fmt"
	"os"

	"github.com/coretrace/agent/cmd"
)

// Build-time variables
var (
	version   string = "dev"
	buildTime string = "unknown"
	commitSHA string = "unknown"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
