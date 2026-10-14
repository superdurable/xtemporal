package refreshworkflow

import (
	"context"

	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/definition"
	"github.com/superdurable/xtemporal/common/locks"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/service/history/api"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
)

func Invoke(
	ctx context.Context,
	workflowKey definition.WorkflowKey,
	archetypeID chasm.ArchetypeID,
	shardContext historyi.ShardContext,
	workflowConsistencyChecker api.WorkflowConsistencyChecker,
) (retError error) {
	err := api.ValidateNamespaceUUID(namespace.ID(workflowKey.NamespaceID))
	if err != nil {
		return err
	}

	if archetypeID == chasm.UnspecifiedArchetypeID {
		archetypeID = chasm.WorkflowArchetypeID
	}

	chasmLease, err := workflowConsistencyChecker.GetChasmLease(
		ctx,
		nil,
		workflowKey,
		archetypeID,
		locks.PriorityLow,
	)
	if err != nil {
		return err
	}
	defer func() { chasmLease.GetReleaseFn()(retError) }()

	return chasmLease.GetContext().RefreshTasks(ctx, shardContext)
}
