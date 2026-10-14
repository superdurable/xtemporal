//go:build test_dep

package tests

import (
	"github.com/superdurable/xtemporal/chasm"
	"go.uber.org/fx"
)

var Module = fx.Module(
	"chasm.lib.tests",
	fx.Invoke(func(registry *chasm.Registry) error {
		return registry.Register(Library)
	}),
)
