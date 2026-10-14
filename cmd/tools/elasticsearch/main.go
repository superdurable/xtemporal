package main

import (
	"os"

	"github.com/superdurable/xtemporal/tools/elasticsearch"
)

func main() {
	if err := elasticsearch.RunTool(os.Args); err != nil {
		os.Exit(1)
	}
}
