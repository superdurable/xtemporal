package tdbg

import (
	"github.com/superdurable/xtemporal/chasm"
	activitylib "github.com/superdurable/xtemporal/chasm/lib/activity"
	callbacklib "github.com/superdurable/xtemporal/chasm/lib/callback"
	chasmscheduler "github.com/superdurable/xtemporal/chasm/lib/scheduler"
	chasmtests "github.com/superdurable/xtemporal/chasm/lib/tests"
	chasmworkflow "github.com/superdurable/xtemporal/chasm/lib/workflow"
	"github.com/superdurable/xtemporal/common/log"
)

func newChasmRegistry(logger log.Logger) (*chasm.Registry, error) {
	registry := chasm.NewRegistry(logger)

	if err := registry.Register(&chasm.CoreLibrary{}); err != nil {
		return nil, err
	}

	if err := registry.Register(chasmworkflow.NewLibrary(chasmworkflow.NewRegistry())); err != nil {
		return nil, err
	}

	if err := registry.Register(activitylib.NewNilLibrary()); err != nil {
		return nil, err
	}

	if err := registry.Register(chasmscheduler.NewNilLibrary()); err != nil {
		return nil, err
	}

	if err := registry.Register(chasmtests.Library); err != nil {
		return nil, err
	}

	if err := registry.Register(callbacklib.NewNilLibrary()); err != nil {
		return nil, err
	}

	return registry, nil
}
