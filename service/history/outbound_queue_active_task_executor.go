package history

import (
	"context"
	"fmt"
	"time"

	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/debug"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/log/tag"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/resource"
	"github.com/superdurable/xtemporal/common/workercommands"
	"github.com/superdurable/xtemporal/service/history/consts"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
	"github.com/superdurable/xtemporal/service/history/queues"
	queueserrors "github.com/superdurable/xtemporal/service/history/queues/errors"
	"github.com/superdurable/xtemporal/service/history/tasks"
	wcache "github.com/superdurable/xtemporal/service/history/workflow/cache"
	enumspb "go.temporal.io/api/enums/v1"
)

const (
	outboundTaskTimeout = time.Second * 10 * debug.TimeoutMultiplier
)

type outboundQueueActiveTaskExecutor struct {
	stateMachineEnvironment
	chasmEngine              chasm.Engine
	workerCommandsDispatcher *workercommands.Dispatcher
}

var _ queues.Executor = &outboundQueueActiveTaskExecutor{}

func newOutboundQueueActiveTaskExecutor(
	shardCtx historyi.ShardContext,
	workflowCache wcache.Cache,
	logger log.Logger,
	metricsHandler metrics.Handler,
	chasmEngine chasm.Engine,
	matchingClient resource.MatchingClient,
) *outboundQueueActiveTaskExecutor {
	scopedMetricsHandler := metricsHandler.WithTags(
		metrics.OperationTag(metrics.OperationOutboundQueueProcessorScope),
	)
	return &outboundQueueActiveTaskExecutor{
		stateMachineEnvironment: stateMachineEnvironment{
			shardContext:   shardCtx,
			cache:          workflowCache,
			logger:         logger,
			metricsHandler: scopedMetricsHandler,
		},
		chasmEngine: chasmEngine,
		workerCommandsDispatcher: workercommands.NewDispatcher(
			matchingClient,
			shardCtx.GetConfig(),
			scopedMetricsHandler,
			logger,
		),
	}
}

func (e *outboundQueueActiveTaskExecutor) Execute(
	ctx context.Context,
	executable queues.Executable,
) queues.ExecuteResponse {
	task := executable.GetTask()
	namespaceTag, replicationState := getNamespaceTagAndReplicationStateByID(
		e.shardContext.GetNamespaceRegistry(),
		task.GetNamespaceID(),
		executable.GetWorkflowID(),
	)
	taskType := queues.GetOutboundTaskTypeTagValue(task, true, e.shardContext.ChasmRegistry())
	respond := func(err error) queues.ExecuteResponse {
		metricsTags := []metrics.Tag{
			namespaceTag,
			metrics.TaskTypeTag(taskType),
			metrics.OperationTag(taskType),
		}
		return queues.ExecuteResponse{
			ExecutionMetricTags: metricsTags,
			ExecutedAsActive:    true,
			ExecutionErr:        err,
		}
	}

	// We don't want to execute outbound tasks when handing over a namespace to avoid starting work that may not be
	// committed and cause duplicate requests.
	// We check namespace handover state **once** when processing is started. Outbound tasks may take up to 10
	// seconds (by default), but we avoid checking again later, before committing the result, to attempt to commit
	// results of inflight tasks and not lose the progress.
	if replicationState == enumspb.REPLICATION_STATE_HANDOVER {
		// TODO: Move this logic to queues.Executable when metrics tags don't need
		// to be returned from task executor. Also check the standby queue logic.
		return respond(consts.ErrNamespaceHandover)
	}

	if err := validateTaskByClock(e.shardContext, task); err != nil {
		return respond(err)
	}

	switch task := task.(type) {
	case *tasks.StateMachineOutboundTask:
		return respond(e.executeStateMachineTask(ctx, task))
	case *tasks.ChasmTask:
		task.Attempt = executable.Attempt()
		return respond(e.executeChasmSideEffectTask(ctx, task))
	case *tasks.WorkerCommandsTask:
		if executable.Attempt() > workercommands.MaxTaskAttempts {
			e.logger.Info("Worker commands task exceeded max attempts, dropping",
				tag.WorkflowID(task.WorkflowID),
				tag.WorkflowRunID(task.RunID),
				tag.NewStringTag("control_queue", task.Destination),
				tag.Attempt(int32(executable.Attempt())),
			)
			workercommands.RecordCommandMetrics(task.Commands, e.metricsHandler, namespaceTag.Value, "max_attempts_exceeded")
			return respond(nil)
		}
		return respond(e.workerCommandsDispatcher.Execute(ctx, task, namespaceTag.Value))
	}

	return respond(queueserrors.NewUnprocessableTaskError(fmt.Sprintf("unknown task type '%T'", task)))
}

func (e *outboundQueueActiveTaskExecutor) executeChasmSideEffectTask(
	ctx context.Context,
	task *tasks.ChasmTask,
) error {
	ctx, cancel := context.WithTimeout(ctx, outboundTaskTimeout)
	defer cancel()

	weContext, release, err := getWorkflowExecutionContextForTask(ctx, e.shardContext, e.cache, task)
	if err != nil {
		return err
	}
	defer func() { release(err) }()

	ms, err := loadMutableStateForTransferTask(ctx, e.shardContext, weContext, task, e.metricsHandler, e.logger)
	if err != nil {
		return err
	}
	tree := ms.ChasmTree()

	// Now that we've loaded the CHASM tree, we can release the lock before task
	// execution. The task's executor must do its own locking as needed, and additional
	// mutable state validations will run at access time.
	release(nil)

	err = executeChasmSideEffectTask(
		ctx,
		e.chasmEngine,
		tree,
		task,
	)
	return err
}

func (e *outboundQueueActiveTaskExecutor) executeStateMachineTask(
	ctx context.Context,
	task tasks.Task,
) error {
	// Timeout for hsm outbound tasks are determined by each component's task executor

	ref, smt, err := StateMachineTask(e.shardContext.StateMachineRegistry(), task)
	if err != nil {
		return err
	}

	smRegistry := e.shardContext.StateMachineRegistry()
	return smRegistry.ExecuteImmediateTask(ctx, e, ref, smt)
}
