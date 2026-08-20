package verifyfirstworkflowtaskscheduled

import (
	"context"

	enumsspb "github.com/superdurable/temporal-server/api/enums/v1"
	"github.com/superdurable/temporal-server/api/historyservice/v1"
	"github.com/superdurable/temporal-server/common/definition"
	"github.com/superdurable/temporal-server/common/locks"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/service/history/api"
	"github.com/superdurable/temporal-server/service/history/consts"
)

func Invoke(
	ctx context.Context,
	req *historyservice.VerifyFirstWorkflowTaskScheduledRequest,
	workflowConsistencyChecker api.WorkflowConsistencyChecker,
) (retError error) {
	namespaceID := namespace.ID(req.GetNamespaceId())
	if err := api.ValidateNamespaceUUID(namespaceID); err != nil {
		return err
	}

	workflowLease, err := workflowConsistencyChecker.GetWorkflowLease(
		ctx,
		req.Clock,
		definition.NewWorkflowKey(
			req.NamespaceId,
			req.WorkflowExecution.WorkflowId,
			req.WorkflowExecution.RunId,
		),
		locks.PriorityLow,
	)
	if err != nil {
		return err
	}
	defer func() { workflowLease.GetReleaseFn()(retError) }()

	mutableState := workflowLease.GetMutableState()
	if !mutableState.IsWorkflowExecutionRunning() &&
		mutableState.GetExecutionState().State != enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE {
		return nil
	}

	if !mutableState.HadOrHasWorkflowTask() {
		return consts.ErrWorkflowNotReady
	}

	return nil
}
