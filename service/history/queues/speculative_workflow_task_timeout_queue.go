package queues

import (
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/common/clock"
	"github.com/superdurable/temporal-server/common/cluster"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/namespace"
	ctasks "github.com/superdurable/temporal-server/common/tasks"
	"github.com/superdurable/temporal-server/service/history/tasks"
	"go.opentelemetry.io/otel/trace"
)

type (
	SpeculativeWorkflowTaskTimeoutQueue struct {
		timeoutQueue     *memoryScheduledQueue
		executor         Executor // *timerQueueActiveTaskExecutor
		priorityAssigner PriorityAssigner

		namespaceRegistry namespace.Registry
		clusterMetadata   cluster.Metadata
		timeSource        clock.TimeSource
		chasmRegistry     *chasm.Registry
		metricsHandler    metrics.Handler
		tracer            trace.Tracer
		logger            log.SnTaggedLogger
	}
)

func NewSpeculativeWorkflowTaskTimeoutQueue(
	scheduler ctasks.Scheduler[ctasks.Task],
	priorityAssigner PriorityAssigner,
	executor Executor,
	namespaceRegistry namespace.Registry,
	clusterMetadata cluster.Metadata,
	timeSource clock.TimeSource,
	chasmRegistry *chasm.Registry,
	metricsHandler metrics.Handler,
	tracer trace.Tracer,
	logger log.SnTaggedLogger,
) *SpeculativeWorkflowTaskTimeoutQueue {

	timeoutQueue := newMemoryScheduledQueue(
		scheduler,
		timeSource,
		logger,
		metricsHandler,
	)

	return &SpeculativeWorkflowTaskTimeoutQueue{
		timeoutQueue:      timeoutQueue,
		executor:          executor,
		priorityAssigner:  priorityAssigner,
		namespaceRegistry: namespaceRegistry,
		clusterMetadata:   clusterMetadata,
		timeSource:        timeSource,
		chasmRegistry:     chasmRegistry,
		metricsHandler:    metricsHandler,
		tracer:            tracer,
		logger:            logger,
	}
}

func (q SpeculativeWorkflowTaskTimeoutQueue) Start() {
	q.timeoutQueue.Start()
}

func (q SpeculativeWorkflowTaskTimeoutQueue) Stop() {
	q.timeoutQueue.Stop()
}

func (q SpeculativeWorkflowTaskTimeoutQueue) Category() tasks.Category {
	return tasks.CategoryMemoryTimer
}

func (q SpeculativeWorkflowTaskTimeoutQueue) NotifyNewTasks(ts []tasks.Task) {
	for _, task := range ts {
		if wttt, ok := task.(*tasks.WorkflowTaskTimeoutTask); ok {
			executable := newSpeculativeWorkflowTaskTimeoutExecutable(NewExecutable(
				0,
				wttt,
				q.executor,
				nil,
				nil,
				q.priorityAssigner,
				q.timeSource,
				q.namespaceRegistry,
				q.clusterMetadata,
				q.chasmRegistry,
				GetTaskTypeTagValue,
				q.logger,
				q.metricsHandler,
				q.tracer,
			), wttt)
			q.timeoutQueue.Add(executable)
		}
	}
}

func (q SpeculativeWorkflowTaskTimeoutQueue) FailoverNamespace(_ string) {
}
