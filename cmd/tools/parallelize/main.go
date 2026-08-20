package main

import (
	"fmt"
	"os"

	"github.com/superdurable/temporal-server/tools/parallelize"
)

func main() {
	if err := parallelize.Main(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
