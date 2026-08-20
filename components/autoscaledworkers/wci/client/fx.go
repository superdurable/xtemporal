package client

import (
	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/persistence/visibility/manager"
	"github.com/superdurable/temporal-server/common/primitives"
	"github.com/superdurable/temporal-server/common/resource"
	"github.com/superdurable/temporal-server/common/testing/testhooks"
	"github.com/superdurable/temporal-server/service/matching/hooks"
	"go.uber.org/fx"
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
