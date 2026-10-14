package main

import (
	"fmt"
	"os"

	"github.com/superdurable/xtemporal/tools/flakereport"
)

func main() {
	if err := flakereport.NewCliApp().Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error running flakereport: %v\n", err)
		os.Exit(1)
	}
}
