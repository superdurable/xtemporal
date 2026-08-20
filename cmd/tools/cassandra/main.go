package main

import (
	"os"

	"github.com/superdurable/temporal-server/tools/cassandra"
)

func main() {
	if err := cassandra.RunTool(os.Args); err != nil {
		os.Exit(1)
	}
}
