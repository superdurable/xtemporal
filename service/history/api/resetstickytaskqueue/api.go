package resetstickytaskqueue

import (
	"context"

	"github.com/superdurable/temporal-server/api/historyservice/v1"
	"github.com/superdurable/temporal-server/common/definition"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/service/history/api"
	"github.com/superdurable/temporal-server/service/history/consts"
	historyi "github.com/superdurable/temporal-server/service/history/interfaces"
)

func Invoke(
	ctx context.Context,
	resetRequest *historyservice.ResetStickyTaskQueueRequest,
	shardContext historyi.ShardContext,
	workflowConsistencyChecker api.WorkflowConsistencyChecker,
) (*historyservice.ResetStickyTaskQueueResponse, error) {
	namespaceID := namespace.ID(resetRequest.GetNamespaceId())
	err := api.ValidateNamespaceUUID(namespaceID)
	if err != nil {
		return nil, err
	}

	err = api.GetAndUpdateWorkflowWithNew(
		ctx,
		nil,
		definition.NewWorkflowKey(
			resetRequest.NamespaceId,
			resetRequest.Execution.WorkflowId,
			resetRequest.Execution.RunId,
		),
		func(workflowLease api.WorkflowLease) (*api.UpdateWorkflowAction, error) {
			mutableState := workflowLease.GetMutableState()
			if !mutableState.IsWorkflowExecutionRunning() {
				return nil, consts.ErrWorkflowCompleted
			}

			mutableState.ClearStickyTaskQueue()
			return &api.UpdateWorkflowAction{
				Noop:               true,
				CreateWorkflowTask: false,
			}, nil
		},
		nil,
		shardContext,
		workflowConsistencyChecker,
	)

	if err != nil {
		return nil, err
	}
	return &historyservice.ResetStickyTaskQueueResponse{}, nil
}
