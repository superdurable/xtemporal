package removesignalmutablestate

import (
	"context"

	"github.com/superdurable/xtemporal/api/historyservice/v1"
	"github.com/superdurable/xtemporal/common/definition"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/service/history/api"
	"github.com/superdurable/xtemporal/service/history/consts"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
)

func Invoke(
	ctx context.Context,
	req *historyservice.RemoveSignalMutableStateRequest,
	shard historyi.ShardContext,
	workflowConsistencyChecker api.WorkflowConsistencyChecker,
) (resp *historyservice.RemoveSignalMutableStateResponse, retError error) {
	_, err := api.GetActiveNamespace(shard, namespace.ID(req.GetNamespaceId()), req.WorkflowExecution.WorkflowId)
	if err != nil {
		return nil, err
	}

	err = api.GetAndUpdateWorkflowWithNew(
		ctx,
		nil,
		definition.NewWorkflowKey(
			req.NamespaceId,
			req.WorkflowExecution.WorkflowId,
			req.WorkflowExecution.RunId,
		),
		func(workflowLease api.WorkflowLease) (*api.UpdateWorkflowAction, error) {
			mutableState := workflowLease.GetMutableState()
			if !mutableState.IsWorkflowExecutionRunning() {
				return nil, consts.ErrWorkflowCompleted
			}

			mutableState.DeleteSignalRequested(req.GetRequestId())
			// This is a no-op if signals are stored in CHASM, since we'll ignore the deletion request,
			// see DeleteSignalRequested implementation.
			// TODO(long-nt-tran): Clean up code once ChasmSignalBacklinksEnabled is fully rolled out.
			return &api.UpdateWorkflowAction{
				Noop:               mutableState.ChasmSignalBacklinksEnabled(),
				CreateWorkflowTask: false,
			}, nil
		},
		nil,
		shard,
		workflowConsistencyChecker,
	)
	if err != nil {
		return nil, err
	}
	return &historyservice.RemoveSignalMutableStateResponse{}, nil
}
