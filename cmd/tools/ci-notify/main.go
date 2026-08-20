package main

import (
	"os"

	cinotify "github.com/superdurable/temporal-server/tools/ci-notify"
)

func main() {
	app := cinotify.NewCliApp()
	if err := app.Run(os.Args); err != nil {
		os.Exit(1)
	}
}
