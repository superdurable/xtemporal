package main

import (
	"os"

	"github.com/superdurable/xtemporal/tools/cassandra"
)

func main() {
	if err := cassandra.RunTool(os.Args); err != nil {
		os.Exit(1)
	}
}
