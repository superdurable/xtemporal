package main

import (
	"fmt"
	"os"

	optimizetestsharding "github.com/superdurable/xtemporal/tools/optimize-test-sharding"
)

func main() {
	if err := optimizetestsharding.Main(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
