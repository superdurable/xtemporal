package queues

import (
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/common/clock"
	"github.com/superdurable/temporal-server/common/cluster"
	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/service/history/tasks"
	"go.opentelemetry.io/otel/trace"
)

type (
	// ExecutableFactory and the related interfaces here are needed to make it possible to extend the functionality of
	// task processing without changing its core logic. This is similar to ExecutorWrapper.
	ExecutableFactory interface {
		NewExecutable(task tasks.Task, readerID int64) Executable
	}
	// ExecutableFactoryFn is a convenience type to avoid having to create a struct that implements ExecutableFactory.
	ExecutableFactoryFn   func(readerID int64, t tasks.Task) Executable
	executableFactoryImpl struct {
		executor                   Executor
		scheduler                  Scheduler
		rescheduler                Rescheduler
		priorityAssigner           PriorityAssigner
		timeSource                 clock.TimeSource
		namespaceRegistry          namespace.Registry
		clusterMetadata            cluster.Metadata
		chasmRegistry              *chasm.Registry
		taskTypeTagProvider        TaskTypeTagProvider
		logger                     log.Logger
		metricsHandler             metrics.Handler
		tracer                     trace.Tracer
		dlqWriter                  *DLQWriter
		dlqEnabled                 dynamicconfig.BoolPropertyFn
		attemptsBeforeSendingToDlq dynamicconfig.IntPropertyFn
		dlqInternalErrors          dynamicconfig.BoolPropertyFn
		dlqErrorPattern            dynamicconfig.StringPropertyFn
	}
)

func (f ExecutableFactoryFn) NewExecutable(task tasks.Task, readerID int64) Executable {
	return f(readerID, task)
}

func NewExecutableFactory(
	executor Executor,
	scheduler Scheduler,
	rescheduler Rescheduler,
	priorityAssigner PriorityAssigner,
	timeSource clock.TimeSource,
	namespaceRegistry namespace.Registry,
	clusterMetadata cluster.Metadata,
	chasmRegistry *chasm.Registry,
	taskTypeTagProvider TaskTypeTagProvider,
	logger log.Logger,
	metricsHandler metrics.Handler,
	tracer trace.Tracer,
	dlqWriter *DLQWriter,
	dlqEnabled dynamicconfig.BoolPropertyFn,
	attemptsBeforeSendingToDlq dynamicconfig.IntPropertyFn,
	dlqInternalErrors dynamicconfig.BoolPropertyFn,
	dlqErrorPattern dynamicconfig.StringPropertyFn,
) *executableFactoryImpl {
	return &executableFactoryImpl{
		executor:                   executor,
		scheduler:                  scheduler,
		rescheduler:                rescheduler,
		priorityAssigner:           priorityAssigner,
		timeSource:                 timeSource,
		namespaceRegistry:          namespaceRegistry,
		clusterMetadata:            clusterMetadata,
		chasmRegistry:              chasmRegistry,
		taskTypeTagProvider:        taskTypeTagProvider,
		logger:                     logger,
		metricsHandler:             metricsHandler,
		tracer:                     tracer,
		dlqWriter:                  dlqWriter,
		dlqEnabled:                 dlqEnabled,
		attemptsBeforeSendingToDlq: attemptsBeforeSendingToDlq,
		dlqInternalErrors:          dlqInternalErrors,
		dlqErrorPattern:            dlqErrorPattern,
	}
}

func (f *executableFactoryImpl) NewExecutable(task tasks.Task, readerID int64) Executable {
	return NewExecutable(
		readerID,
		task,
		f.executor,
		f.scheduler,
		f.rescheduler,
		f.priorityAssigner,
		f.timeSource,
		f.namespaceRegistry,
		f.clusterMetadata,
		f.chasmRegistry,
		f.taskTypeTagProvider,
		f.logger,
		f.metricsHandler,
		f.tracer,
		func(params *ExecutableParams) {
			params.DLQEnabled = f.dlqEnabled
			params.DLQWriter = f.dlqWriter
			params.MaxUnexpectedErrorAttempts = f.attemptsBeforeSendingToDlq
			params.DLQInternalErrors = f.dlqInternalErrors
			params.DLQErrorPattern = f.dlqErrorPattern
		},
	)
}
