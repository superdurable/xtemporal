package tdbgtest

import (
	"github.com/superdurable/xtemporal/tools/tdbg"
	"github.com/urfave/cli/v2"
)

// NewCliApp is a wrapper around [tdbg.NewCliApp] that sets the [cli.App.ExitErrHandler] to a no-op function.
func NewCliApp(opts ...tdbg.Option) *cli.App {
	app := tdbg.NewCliApp(opts...)
	app.ExitErrHandler = func(context *cli.Context, err error) {}
	return app
}
