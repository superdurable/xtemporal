package describemutablestate

import (
	"context"

	"github.com/superdurable/xtemporal/api/historyservice/v1"
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/definition"
	"github.com/superdurable/xtemporal/common/locks"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/service/history/api"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
	"github.com/superdurable/xtemporal/service/history/workflow"
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
