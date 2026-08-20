package describemutablestate

import (
	"context"

	"github.com/superdurable/temporal-server/api/historyservice/v1"
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/common/definition"
	"github.com/superdurable/temporal-server/common/locks"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/service/history/api"
	historyi "github.com/superdurable/temporal-server/service/history/interfaces"
	"github.com/superdurable/temporal-server/service/history/workflow"
)

func Invoke(
	ctx context.Context,
	req *historyservice.DescribeMutableStateRequest,
	shardContext historyi.ShardContext,
	workflowConsistencyChecker api.WorkflowConsistencyChecker,
) (_ *historyservice.DescribeMutableStateResponse, retError error) {
	namespaceID := namespace.ID(req.GetNamespaceId())
	err := api.ValidateNamespaceUUID(namespaceID)
	if err != nil {
		return nil, err
	}

	archetypeID := req.GetArchetypeId()
	if archetypeID == chasm.UnspecifiedArchetypeID {
		archetypeID = chasm.WorkflowArchetypeID
	}

	chasmLease, err := workflowConsistencyChecker.GetChasmLease(
		ctx,
		nil,
		definition.NewWorkflowKey(
			req.NamespaceId,
			req.Execution.WorkflowId,
			req.Execution.RunId,
		),
		archetypeID,
		locks.PriorityHigh,
	)
	if err != nil {
		return nil, err
	}
	defer func() { chasmLease.GetReleaseFn()(retError) }()

	response := &historyservice.DescribeMutableStateResponse{}
	if chasmLease.GetContext().(*workflow.ContextImpl).MutableState != nil {
		msb := chasmLease.GetContext().(*workflow.ContextImpl).MutableState
		response.CacheMutableState = msb.CloneToProto()
	}

	if !req.GetSkipForceReload() {
		// Clear mutable state to force reload from persistence.
		chasmLease.GetContext().Clear()
	}

	mutableState, err := chasmLease.GetContext().LoadMutableState(ctx, shardContext)
	if err != nil {
		return nil, err
	}

	response.DatabaseMutableState = mutableState.CloneToProto()
	return response, nil
}
