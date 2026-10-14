package main

import (
	"os"

	"github.com/superdurable/xtemporal/tools/tdbg"
)

func main() {
	app := tdbg.NewCliApp()
	if err := app.Run(os.Args); err != nil {
		os.Exit(1)
	}
}
