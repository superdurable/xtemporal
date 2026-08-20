package isworkflowtaskvalid

import (
	"context"

	"github.com/superdurable/temporal-server/api/historyservice/v1"
	"github.com/superdurable/temporal-server/common"
	"github.com/superdurable/temporal-server/common/definition"
	"github.com/superdurable/temporal-server/service/history/api"
	"github.com/superdurable/temporal-server/service/history/consts"
	historyi "github.com/superdurable/temporal-server/service/history/interfaces"
)

func Invoke(
	ctx context.Context,
	req *historyservice.IsWorkflowTaskValidRequest,
	shardContext historyi.ShardContext,
	workflowConsistencyChecker api.WorkflowConsistencyChecker,
) (resp *historyservice.IsWorkflowTaskValidResponse, retError error) {
	isValid := false
	err := api.GetAndUpdateWorkflowWithNew(
		ctx,
		req.Clock,
		definition.NewWorkflowKey(
			req.NamespaceId,
			req.Execution.WorkflowId,
			req.Execution.RunId,
		),
		func(workflowLease api.WorkflowLease) (*api.UpdateWorkflowAction, error) {
			isTaskValid, err := isWorkflowTaskValid(workflowLease, req.ScheduledEventId, req.GetStamp())
			if err != nil {
				return nil, err
			}
			isValid = isTaskValid
			return &api.UpdateWorkflowAction{
				Noop:               true,
				CreateWorkflowTask: false,
			}, nil
		},
		nil,
		shardContext,
		workflowConsistencyChecker,
	)
	return &historyservice.IsWorkflowTaskValidResponse{
		IsValid: isValid,
	}, err
}

func isWorkflowTaskValid(
	workflowLease api.WorkflowLease,
	scheduledEventID int64,
	stamp int32,
) (bool, error) {
	mutableState := workflowLease.GetMutableState()
	if !mutableState.IsWorkflowExecutionRunning() {
		return false, consts.ErrWorkflowCompleted
	}

	workflowTask := mutableState.GetWorkflowTaskByID(scheduledEventID)
	if workflowTask == nil {
		return false, nil
	}
	if stamp != workflowTask.Stamp {
		// This happens when the workflow task was rescheduled.
		return false, nil
	}
	return workflowTask.StartedEventID == common.EmptyEventID, nil
}
