package refreshworkflow

import (
	"context"

	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/common/definition"
	"github.com/superdurable/temporal-server/common/locks"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/service/history/api"
	historyi "github.com/superdurable/temporal-server/service/history/interfaces"
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
