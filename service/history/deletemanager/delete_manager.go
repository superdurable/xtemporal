//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination delete_manager_mock.go

package deletemanager

import (
	"context"

	commonpb "go.temporal.io/api/common/v1"
	"github.com/superdurable/xtemporal/common/clock"
	"github.com/superdurable/xtemporal/common/definition"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/common/persistence/visibility/manager"
	"github.com/superdurable/xtemporal/service/history/configs"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
	"github.com/superdurable/xtemporal/service/history/tasks"
	"github.com/superdurable/xtemporal/service/history/workflow"
	wcache "github.com/superdurable/xtemporal/service/history/workflow/cache"
)

type (
	DeleteManager interface {
		AddDeleteExecutionTask(
			ctx context.Context,
			nsID namespace.ID,
			we *commonpb.WorkflowExecution,
			ms historyi.MutableState,
		) error
		DeleteWorkflowExecution(
			ctx context.Context,
			nsID namespace.ID,
			we *commonpb.WorkflowExecution,
			weCtx historyi.WorkflowContext,
			ms historyi.MutableState,
			stage *tasks.DeleteWorkflowExecutionStage,
		) error
		DeleteWorkflowExecutionByRetention(
			ctx context.Context,
			nsID namespace.ID,
			we *commonpb.WorkflowExecution,
			weCtx historyi.WorkflowContext,
			ms historyi.MutableState,
			stage *tasks.DeleteWorkflowExecutionStage,
		) error
	}

	DeleteManagerImpl struct {
		shardContext      historyi.ShardContext
		workflowCache     wcache.Cache
		config            *configs.Config
		metricsHandler    metrics.Handler
		timeSource        clock.TimeSource
		visibilityManager manager.VisibilityManager
	}
)

var _ DeleteManager = (*DeleteManagerImpl)(nil)

func NewDeleteManager(
	shardContext historyi.ShardContext,
	cache wcache.Cache,
	config *configs.Config,
	timeSource clock.TimeSource,
	visibilityManager manager.VisibilityManager,
) *DeleteManagerImpl {
	deleteManager := &DeleteManagerImpl{
		shardContext:      shardContext,
		workflowCache:     cache,
		metricsHandler:    shardContext.GetMetricsHandler(),
		config:            config,
		timeSource:        timeSource,
		visibilityManager: visibilityManager,
	}

	return deleteManager
}

func (m *DeleteManagerImpl) AddDeleteExecutionTask(
	ctx context.Context,
	nsID namespace.ID,
	we *commonpb.WorkflowExecution,
	ms historyi.MutableState,
) error {
	return AddDeleteExecutionTask(ctx, m.shardContext, nsID, we, ms)
}

// AddDeleteExecutionTask creates a DeleteExecutionTask and adds it to the shard's task queue.
// This is a package-level function so it can be used by callers that don't have a DeleteManager instance.
func AddDeleteExecutionTask(
	ctx context.Context,
	shardContext historyi.ShardContext,
	nsID namespace.ID,
	we *commonpb.WorkflowExecution,
	ms historyi.MutableState,
) error {
	taskGenerator := workflow.GetTaskGeneratorProvider().NewTaskGenerator(shardContext, ms)

	// We can make this task immediately because the task itself will keep rescheduling itself until the
	// execution is closed before actually deleting it.
	deleteTask, err := taskGenerator.GenerateDeleteExecutionTask()
	if err != nil {
		return err
	}

	return shardContext.AddTasks(ctx, &persistence.AddHistoryTasksRequest{
		ShardID:     shardContext.GetShardID(),
		NamespaceID: nsID.String(),
		WorkflowID:  we.GetWorkflowId(),
		ArchetypeID: ms.ChasmTree().ArchetypeID(),
		Tasks: map[tasks.Category][]tasks.Task{
			tasks.CategoryTransfer: {deleteTask},
		},
	})
}

func (m *DeleteManagerImpl) DeleteWorkflowExecution(
	ctx context.Context,
	nsID namespace.ID,
	we *commonpb.WorkflowExecution,
	weCtx historyi.WorkflowContext,
	ms historyi.MutableState,
	stage *tasks.DeleteWorkflowExecutionStage,
) error {

	return m.deleteWorkflowExecutionInternal(
		ctx,
		nsID,
		we,
		weCtx,
		ms,
		stage,
		false,
		m.metricsHandler.WithTags(metrics.OperationTag(metrics.HistoryDeleteWorkflowExecutionScope)),
	)
}

func (m *DeleteManagerImpl) DeleteWorkflowExecutionByRetention(
	ctx context.Context,
	nsID namespace.ID,
	we *commonpb.WorkflowExecution,
	weCtx historyi.WorkflowContext,
	ms historyi.MutableState,
	stage *tasks.DeleteWorkflowExecutionStage,
) error {
	// Skip replication for retention-based deletion. Both clusters have independent retention
	// timers and will delete on their own schedule.
	stage.MarkProcessed(tasks.DeleteWorkflowExecutionStageReplication)

	return m.deleteWorkflowExecutionInternal(
		ctx,
		nsID,
		we,
		weCtx,
		ms,
		stage,
		true,
		m.metricsHandler.WithTags(metrics.OperationTag(metrics.HistoryProcessDeleteHistoryEventScope)),
	)
}

func (m *DeleteManagerImpl) deleteWorkflowExecutionInternal(
	ctx context.Context,
	namespaceID namespace.ID,
	we *commonpb.WorkflowExecution,
	weCtx historyi.WorkflowContext,
	ms historyi.MutableState,
	stage *tasks.DeleteWorkflowExecutionStage,
	retentionDelete bool,
	metricsHandler metrics.Handler,
) error {

	currentBranchToken, err := ms.GetCurrentBranchToken()
	if err != nil {
		return err
	}

	closeTime, err := ms.GetWorkflowCloseTime(ctx)
	if err != nil {
		return err
	}

	executionInfo := ms.GetExecutionInfo()
	if err := m.shardContext.DeleteWorkflowExecution(
		ctx,
		definition.WorkflowKey{
			NamespaceID: namespaceID.String(),
			WorkflowID:  we.GetWorkflowId(),
			RunID:       we.GetRunId(),
		},
		ms.ChasmTree().ArchetypeID(),
		currentBranchToken,
		executionInfo.GetCloseVisibilityTaskId(),
		closeTime,
		ms.GetExecutionState().GetStartTime().AsTime(),
		stage,
		retentionDelete,
	); err != nil {
		return err
	}

	// Clear workflow execution context here to prevent further readers to get stale copy of non-exiting workflow execution.
	weCtx.Clear()

	metrics.WorkflowCleanupDeleteCount.With(metricsHandler).Record(1)
	return nil
}
