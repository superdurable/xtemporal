package tdbg

import (
	"github.com/superdurable/temporal-server/chasm"
	chasmscheduler "github.com/superdurable/temporal-server/chasm/lib/scheduler"
	chasmtests "github.com/superdurable/temporal-server/chasm/lib/tests"
	chasmworkflow "github.com/superdurable/temporal-server/chasm/lib/workflow"
	"github.com/superdurable/temporal-server/common/log"
)

func newChasmRegistry(logger log.Logger) (*chasm.Registry, error) {
	registry := chasm.NewRegistry(logger)

	if err := registry.Register(&chasm.CoreLibrary{}); err != nil {
		return nil, err
	}

	if err := registry.Register(chasmworkflow.NewLibrary()); err != nil {
		return nil, err
	}

	if err := registry.Register(chasmscheduler.NewNilLibrary()); err != nil {
		return nil, err
	}

	if err := registry.Register(chasmtests.Library); err != nil {
		return nil, err
	}

	// Note: Activity and Callback libraries are not included because their constructors
	// are unexported. Add them if/when they're needed.

	return registry, nil
}
