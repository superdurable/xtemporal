package history

import (
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/log/tag"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/persistence/visibility/manager"
	ctasks "github.com/superdurable/xtemporal/common/tasks"
	"github.com/superdurable/xtemporal/common/telemetry"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
	"github.com/superdurable/xtemporal/service/history/queues"
	"github.com/superdurable/xtemporal/service/history/tasks"
	"go.uber.org/fx"
)

const (
	visibilityQueuePersistenceMaxRPSRatio = 0.15
)

type (
	visibilityQueueFactoryParams struct {
		fx.In

		QueueFactoryBaseParams

		VisibilityMgr manager.VisibilityManager
	}

	visibilityQueueFactory struct {
		visibilityQueueFactoryParams
		QueueFactoryBase
	}
)

func NewVisibilityQueueFactory(
	params visibilityQueueFactoryParams,
) QueueFactory {
	return &visibilityQueueFactory{
		visibilityQueueFactoryParams: params,
		QueueFactoryBase: QueueFactoryBase{
			HostScheduler: queues.NewScheduler(
				params.ClusterMetadata.GetCurrentClusterName(),
				queues.SchedulerOptions{
					WorkerCount:                    params.Config.VisibilityProcessorSchedulerWorkerCount,
					ActiveNamespaceWeights:         params.Config.VisibilityProcessorSchedulerActiveRoundRobinWeights,
					StandbyNamespaceWeights:        params.Config.VisibilityProcessorSchedulerStandbyRoundRobinWeights,
					InactiveNamespaceDeletionDelay: params.Config.TaskSchedulerInactiveChannelDeletionDelay,
					ExecutionAwareSchedulerOptions: ctasks.ExecutionAwareSchedulerOptions{
						Enabled:          params.Config.TaskSchedulerEnableExecutionQueueScheduler,
						MaxQueues:        params.Config.TaskSchedulerExecutionQueueSchedulerMaxQueues,
						QueueTTL:         params.Config.TaskSchedulerExecutionQueueSchedulerQueueTTL,
						QueueConcurrency: params.Config.TaskSchedulerExecutionQueueSchedulerQueueConcurrency,
					},
				},
				params.NamespaceRegistry,
				params.Logger,
				params.MetricsHandler,
				params.TimeSource,
			),
			HostPriorityAssigner: queues.NewPriorityAssigner(
				params.NamespaceRegistry,
				params.ClusterMetadata.GetCurrentClusterName(),
			),
			HostReaderRateLimiter: queues.NewReaderPriorityRateLimiter(
				NewHostRateLimiterRateFn(
					params.Config.VisibilityProcessorMaxPollHostRPS,
					params.Config.PersistenceMaxQPS,
					visibilityQueuePersistenceMaxRPSRatio,
				),
				int64(params.Config.VisibilityQueueMaxReaderCount()),
			),
			Tracer: params.TracerProvider.Tracer(telemetry.ComponentQueueVisibility),
		},
	}
}

func (f *visibilityQueueFactory) CreateQueue(
	shard historyi.ShardContext,
) queues.Queue {
	logger := log.With(shard.GetLogger(), tag.ComponentVisibilityQueue)
	metricsHandler := f.MetricsHandler.WithTags(metrics.OperationTag(metrics.OperationVisibilityQueueProcessorScope))

	shardScheduler := queues.NewRateLimitedScheduler(
		f.HostScheduler,
		queues.RateLimitedSchedulerOptions{
			Enabled:          f.Config.TaskSchedulerEnableRateLimiter,
			EnableShadowMode: f.Config.TaskSchedulerEnableRateLimiterShadowMode,
			StartupDelay:     f.Config.TaskSchedulerRateLimiterStartupDelay,
		},
		f.ClusterMetadata.GetCurrentClusterName(),
		f.NamespaceRegistry,
		f.SchedulerRateLimiter,
		f.TimeSource,
		f.ChasmRegistry,
		logger,
		metricsHandler,
	)

	rescheduler := queues.NewRescheduler(
		shardScheduler,
		shard.GetTimeSource(),
		logger,
		metricsHandler,
	)

	executor := newVisibilityQueueTaskExecutor(
		shard,
		f.WorkflowCache,
		f.VisibilityMgr,
		logger,
		f.MetricsHandler,
		f.Config.VisibilityProcessorEnsureCloseBeforeDelete,
		f.Config.VisibilityProcessorEnableCloseWorkflowCleanup,
		f.Config.VisibilityProcessorRelocateAttributesMinBlobSize,
		f.Config.ExternalPayloadsEnabled,
	)
	if f.ExecutorWrapper != nil {
		executor = f.ExecutorWrapper.Wrap(executor)
	}

	factory := queues.NewExecutableFactory(
		executor,
		shardScheduler,
		rescheduler,
		f.HostPriorityAssigner,
		shard.GetTimeSource(),
		shard.GetNamespaceRegistry(),
		shard.GetClusterMetadata(),
		f.ChasmRegistry,
		queues.GetTaskTypeTagValue,
		logger,
		metricsHandler,
		f.Tracer,
		f.DLQWriter,
		f.Config.TaskDLQEnabled,
		f.Config.TaskDLQUnexpectedErrorAttempts,
		f.Config.TaskDLQInternalErrors,
		f.Config.TaskDLQErrorPattern,
	)
	return queues.NewImmediateQueue(
		shard,
		tasks.CategoryVisibility,
		shardScheduler,
		rescheduler,
		&queues.Options{
			ReaderOptions: queues.ReaderOptions{
				BatchSize:            f.Config.VisibilityTaskBatchSize,
				MaxPendingTasksCount: f.Config.QueuePendingTaskMaxCount,
				PollBackoffInterval:  f.Config.VisibilityProcessorPollBackoffInterval,
				MaxPredicateSize:     f.Config.QueueMaxPredicateSize,
			},
			MonitorOptions: queues.MonitorOptions{
				PendingTasksCriticalCount:   f.Config.QueuePendingTaskCriticalCount,
				ReaderStuckCriticalAttempts: f.Config.QueueReaderStuckCriticalAttempts,
				SliceCountCriticalThreshold: f.Config.QueueCriticalSlicesCount,
			},
			MaxPollRPS:                          f.Config.VisibilityProcessorMaxPollRPS,
			MaxPollInterval:                     f.Config.VisibilityProcessorMaxPollInterval,
			MaxPollIntervalJitterCoefficient:    f.Config.VisibilityProcessorMaxPollIntervalJitterCoefficient,
			CheckpointInterval:                  f.Config.VisibilityProcessorUpdateAckInterval,
			CheckpointIntervalJitterCoefficient: f.Config.VisibilityProcessorUpdateAckIntervalJitterCoefficient,
			MaxReaderCount:                      f.Config.VisibilityQueueMaxReaderCount,
			MoveGroupTaskCountBase:              f.Config.QueueMoveGroupTaskCountBase,
			MoveGroupTaskCountMultiplier:        f.Config.QueueMoveGroupTaskCountMultiplier,
			ShrinkPredicateMaxPendingKeys:       f.Config.QueueShrinkPredicateMaxPendingKeys,
		},
		f.HostReaderRateLimiter,
		queues.GrouperNamespaceID{},
		logger,
		metricsHandler,
		factory,
		nil, // taskPostProcessor
	)
}
