package history

import (
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/clock"
	"github.com/superdurable/xtemporal/common/cluster"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/log/tag"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/namespace"
	ctasks "github.com/superdurable/xtemporal/common/tasks"
	"github.com/superdurable/xtemporal/common/telemetry"
	"github.com/superdurable/xtemporal/service/history/configs"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
	"github.com/superdurable/xtemporal/service/history/queues"
	wcache "github.com/superdurable/xtemporal/service/history/workflow/cache"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
)

type (
	memoryScheduledQueueFactoryParams struct {
		fx.In

		NamespaceRegistry namespace.Registry
		ClusterMetadata   cluster.Metadata
		WorkflowCache     wcache.Cache
		Config            *configs.Config
		TimeSource        clock.TimeSource
		ChasmRegistry     *chasm.Registry
		MetricsHandler    metrics.Handler
		TracerProvider    trace.TracerProvider
		Logger            log.SnTaggedLogger

		ExecutorWrapper queues.ExecutorWrapper `optional:"true"`
	}

	memoryScheduledQueueFactory struct {
		scheduler        ctasks.Scheduler[ctasks.Task]
		priorityAssigner queues.PriorityAssigner

		namespaceRegistry namespace.Registry
		clusterMetadata   cluster.Metadata
		workflowCache     wcache.Cache
		timeSource        clock.TimeSource
		chasmRegistry     *chasm.Registry
		metricsHandler    metrics.Handler
		tracer            trace.Tracer
		logger            log.SnTaggedLogger

		executorWrapper queues.ExecutorWrapper
	}
)

func NewMemoryScheduledQueueFactory(
	params memoryScheduledQueueFactoryParams,
) QueueFactory {
	logger := log.With(params.Logger, tag.ComponentMemoryScheduledQueue)
	metricsHandler := params.MetricsHandler.WithTags(metrics.OperationTag(metrics.OperationMemoryScheduledQueueProcessorScope))

	hostScheduler := ctasks.NewFIFOScheduler[ctasks.Task](
		&ctasks.FIFOSchedulerOptions{
			QueueSize:   0, // Don't buffer tasks in scheduler. If all workers are busy memoryScheduledQueue reschedules tasks into itself.
			WorkerCount: params.Config.MemoryTimerProcessorSchedulerWorkerCount,
		},
		logger,
	)

	return &memoryScheduledQueueFactory{
		scheduler: hostScheduler,
		priorityAssigner: queues.NewPriorityAssigner(
			params.NamespaceRegistry,
			params.ClusterMetadata.GetCurrentClusterName(),
		),
		namespaceRegistry: params.NamespaceRegistry,
		clusterMetadata:   params.ClusterMetadata,
		workflowCache:     params.WorkflowCache,
		timeSource:        params.TimeSource,
		chasmRegistry:     params.ChasmRegistry,
		metricsHandler:    metricsHandler,
		tracer:            params.TracerProvider.Tracer(telemetry.ComponentQueueMemory),
		logger:            logger,
		executorWrapper:   params.ExecutorWrapper,
	}
}

func (f *memoryScheduledQueueFactory) Start() {
	f.scheduler.Start()
}

func (f *memoryScheduledQueueFactory) Stop() {
	f.scheduler.Stop()
}

func (f *memoryScheduledQueueFactory) CreateQueue(
	shardCtx historyi.ShardContext,
) queues.Queue {

	// Reuse TimerQueueActiveTaskExecutor only to executeWorkflowTaskTimeoutTask.
	// Unused dependencies are nil.
	speculativeWorkflowTaskTimeoutExecutor := newTimerQueueActiveTaskExecutor(
		shardCtx,
		f.workflowCache,
		nil,
		f.logger,
		f.metricsHandler,
		shardCtx.GetConfig(),
		nil,
		nil,
	)
	if f.executorWrapper != nil {
		speculativeWorkflowTaskTimeoutExecutor = f.executorWrapper.Wrap(speculativeWorkflowTaskTimeoutExecutor)
	}

	return queues.NewSpeculativeWorkflowTaskTimeoutQueue(
		f.scheduler,
		f.priorityAssigner,
		speculativeWorkflowTaskTimeoutExecutor,
		f.namespaceRegistry,
		f.clusterMetadata,
		f.timeSource,
		f.chasmRegistry,
		f.metricsHandler,
		f.tracer,
		f.logger,
	)
}
