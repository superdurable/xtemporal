package client

import (
	"go.uber.org/fx"

	"github.com/superdurable/xtemporal/common/dynamicconfig"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/persistence/visibility/manager"
	"github.com/superdurable/xtemporal/common/primitives"
	"github.com/superdurable/xtemporal/common/resource"
	"github.com/superdurable/xtemporal/common/testing/testhooks"
	"github.com/superdurable/xtemporal/service/matching/hooks"
)

var Module = fx.Options(
	fx.Provide(ClientProvider),
)

type (
	fxOut struct {
		fx.Out

		Client          Client
		TaskHookFactory hooks.TaskHookFactory `group:"TaskHookFactories"`
	}
)

func ClientProvider(
	logger log.Logger,
	historyClient resource.HistoryClient,
	visibilityManager manager.VisibilityManager,
	dc *dynamicconfig.Collection,
	testHooks testhooks.TestHooks,
	metricsHandler metrics.Handler,
) fxOut {
	client := &clientImpl{
		logger:                       logger,
		controllerTaskQueueName:      primitives.PerNSWorkerTaskQueue,
		historyClient:                historyClient,
		visibilityManager:            visibilityManager,
		maxIDLengthLimit:             dynamicconfig.MaxIDLengthLimit.Get(dc),
		visibilityMaxPageSize:        dynamicconfig.FrontendVisibilityMaxPageSize.Get(dc),
		maxWorkerControllerInstances: WorkerControllerMaxInstances.Get(dc),
		testHooks:                    testHooks,
		metricsHandler:               metricsHandler,
	}
	taskHookFactory := &taskHookFactoryImpl{
		logger:         logger,
		client:         client,
		dc:             dc,
		metricsHandler: metricsHandler,
	}

	return fxOut{Client: client, TaskHookFactory: taskHookFactory}
}
