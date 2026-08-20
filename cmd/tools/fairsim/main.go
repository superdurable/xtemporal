package main

import (
	"fmt"
	"os"

	"github.com/superdurable/temporal-server/tools/fairsim"
)

func main() {
	if err := fairsim.RunTool(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
