package replication

import (
	"context"
	"errors"
	"time"

	enumsspb "github.com/superdurable/temporal-server/api/enums/v1"
	persistencespb "github.com/superdurable/temporal-server/api/persistence/v1"
	replicationspb "github.com/superdurable/temporal-server/api/replication/v1"
	"github.com/superdurable/temporal-server/common/definition"
	"github.com/superdurable/temporal-server/common/headers"
	"github.com/superdurable/temporal-server/common/log/tag"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/namespace"
	serviceerrors "github.com/superdurable/temporal-server/common/serviceerror"
	ctasks "github.com/superdurable/temporal-server/common/tasks"
	"github.com/superdurable/temporal-server/service/history/consts"
	historyi "github.com/superdurable/temporal-server/service/history/interfaces"
	"go.temporal.io/api/serviceerror"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This is mostly copied from ExecutableActivityStateTask
// The 4 replication executable task implemenatations are quite similar
// we may want to do some refactoring later.

type (
	ExecutableSyncHSMTask struct {
		ProcessToolBox

		definition.WorkflowKey
		ExecutableTask

		taskAttr *replicationspb.SyncHSMAttributes
	}
)

var _ ctasks.Task = (*ExecutableSyncHSMTask)(nil)
var _ TrackableExecutableTask = (*ExecutableSyncHSMTask)(nil)

// var _ BatchableTask = (*ExecutableSyncHSMTask)(nil)

func NewExecutableSyncHSMTask(
	processToolBox ProcessToolBox,
	taskID int64,
	taskCreationTime time.Time,
	task *replicationspb.SyncHSMAttributes,
	sourceClusterName string,
	sourceShardKey ClusterShardKey,
	replicationTask *replicationspb.ReplicationTask,
) *ExecutableSyncHSMTask {
	return &ExecutableSyncHSMTask{
		ProcessToolBox: processToolBox,

		WorkflowKey: definition.NewWorkflowKey(task.NamespaceId, task.WorkflowId, task.RunId),
		ExecutableTask: NewExecutableTask(
			processToolBox,
			taskID,
			metrics.SyncHSMTaskScope,
			taskCreationTime,
			time.Now().UTC(),
			sourceClusterName,
			sourceShardKey,
			replicationTask,
		),
		taskAttr: task,
	}
}

func (e *ExecutableSyncHSMTask) QueueID() any {
	return e.WorkflowKey
}

func (e *ExecutableSyncHSMTask) Execute() error {
	if e.TerminalState() {
		return nil
	}
	e.MarkExecutionStart()

	callerInfo := getReplicaitonCallerInfo(e.GetPriority())
	namespaceName, apply, nsError := e.GetNamespaceInfo(headers.SetCallerInfo(
		context.Background(),
		callerInfo,
	), e.NamespaceID, e.WorkflowID)
	if nsError != nil {
		return nsError
	} else if !apply {
		e.Logger.Warn("Skipping the replication task",
			tag.WorkflowNamespaceID(e.NamespaceID),
			tag.WorkflowID(e.WorkflowID),
			tag.WorkflowRunID(e.RunID),
			tag.TaskID(e.ExecutableTask.TaskID()),
		)
		metrics.ReplicationTasksSkipped.With(e.MetricsHandler).Record(
			1,
			metrics.OperationTag(metrics.SyncHSMTaskScope),
			metrics.NamespaceTag(namespaceName),
		)
		return nil
	}
	ctx, cancel := newTaskContext(namespaceName, e.Config.ReplicationTaskApplyTimeout(), callerInfo)
	defer cancel()

	shardContext, err := e.ShardController.GetShardByNamespaceWorkflow(
		namespace.ID(e.NamespaceID),
		e.WorkflowID,
	)
	if err != nil {
		return err
	}
	engine, err := shardContext.GetEngine(ctx)
	if err != nil {
		return err
	}
	return engine.SyncHSM(ctx, &historyi.SyncHSMRequest{
		WorkflowKey:         e.WorkflowKey,
		StateMachineNode:    e.taskAttr.StateMachineNode,
		EventVersionHistory: e.taskAttr.VersionHistory,
	})

}

func (e *ExecutableSyncHSMTask) HandleErr(err error) error {
	metrics.ReplicationTasksErrorByType.With(e.MetricsHandler).Record(
		1,
		metrics.OperationTag(metrics.SyncHSMTaskScope),
		metrics.NamespaceTag(e.NamespaceName()),
		metrics.ServiceErrorTypeTag(err),
	)
	if errors.Is(err, consts.ErrDuplicate) {
		e.MarkTaskDuplicated()
		return nil
	}
	callerInfo := getReplicaitonCallerInfo(e.GetPriority())
	switch retryErr := err.(type) {
	case nil, *serviceerror.NotFound:
		return nil
	case *serviceerrors.RetryReplication:
		namespaceName, _, nsError := e.GetNamespaceInfo(headers.SetCallerInfo(
			context.Background(),
			callerInfo,
		), e.NamespaceID, e.WorkflowID)
		if nsError != nil {
			return err
		}
		ctx, cancel := newTaskContext(namespaceName, e.Config.ReplicationTaskApplyTimeout(), callerInfo)
		defer cancel()

		if doContinue, resendErr := e.Resend(
			ctx,
			e.ExecutableTask.SourceClusterName(),
			retryErr,
			ResendAttempt,
		); resendErr != nil || !doContinue {
			return err
		}
		return e.Execute()
	default:
		e.Logger.Error("Sync HSM replication task encountered error",
			tag.WorkflowNamespaceID(e.NamespaceID),
			tag.WorkflowID(e.WorkflowID),
			tag.WorkflowRunID(e.RunID),
			tag.TaskID(e.ExecutableTask.TaskID()),
			tag.Error(err),
		)
		return err
	}
}

func (e *ExecutableSyncHSMTask) MarkPoisonPill() error {

	if e.ReplicationTask().GetRawTaskInfo() == nil {
		e.ReplicationTask().RawTaskInfo = &persistencespb.ReplicationTaskInfo{
			NamespaceId:    e.NamespaceID,
			WorkflowId:     e.WorkflowID,
			RunId:          e.RunID,
			TaskType:       enumsspb.TASK_TYPE_REPLICATION_SYNC_HSM,
			TaskId:         e.ExecutableTask.TaskID(),
			VisibilityTime: timestamppb.New(e.TaskCreationTime()),
		}
	}

	return e.ExecutableTask.MarkPoisonPill()
}

// TODO: implement the following methods to batch syncHSM task if needed
// if not implemented the task will be treated as unbatchable

// func (e *ExecutableSyncHSMTask) BatchWith(incomingTask BatchableTask) (TrackableExecutableTask, bool) {
// 	panic("not implemented")
// }

// func (e *ExecutableSyncHSMTask) CanBatch() bool {
// 	panic("not implemented")
// }

// func (e *ExecutableSyncHSMTask) MarkUnbatchable() {
// 	panic("not implemented")
// }
