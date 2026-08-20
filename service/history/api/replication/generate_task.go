package replication

import (
	"context"

	"github.com/superdurable/temporal-server/api/historyservice/v1"
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/common/definition"
	"github.com/superdurable/temporal-server/common/locks"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/common/persistence"
	"github.com/superdurable/temporal-server/service/history/api"
	historyi "github.com/superdurable/temporal-server/service/history/interfaces"
	"github.com/superdurable/temporal-server/service/history/tasks"
)

func GenerateTask(
	ctx context.Context,
	request *historyservice.GenerateLastHistoryReplicationTasksRequest,
	shardContext historyi.ShardContext,
	workflowConsistencyChecker api.WorkflowConsistencyChecker,
) (_ *historyservice.GenerateLastHistoryReplicationTasksResponse, retError error) {
	namespaceEntry, err := api.GetNamespace(shardContext, namespace.ID(request.GetNamespaceId()))
	if err != nil {
		return nil, err
	}
	namespaceID := namespaceEntry.ID()

	archetypeID := request.GetArchetypeId()
	if archetypeID == chasm.UnspecifiedArchetypeID {
		archetypeID = chasm.WorkflowArchetypeID
	}

	chasmLease, err := workflowConsistencyChecker.GetChasmLease(
		ctx,
		nil,
		definition.NewWorkflowKey(
			namespaceID.String(),
			request.Execution.WorkflowId,
			request.Execution.RunId,
		),
		archetypeID,
		locks.PriorityHigh,
	)
	if err != nil {
		return nil, err
	}
	defer func() { chasmLease.GetReleaseFn()(retError) }()

	mutableState := chasmLease.GetMutableState()
	replicationTasks, stateTransitionCount, err := mutableState.GenerateMigrationTasks(request.GetTargetClusters())
	if err != nil {
		return nil, err
	}

	err = shardContext.AddTasks(ctx, &persistence.AddHistoryTasksRequest{
		ShardID: shardContext.GetShardID(),
		// RangeID is set by shard
		NamespaceID: string(namespaceID),
		WorkflowID:  request.Execution.WorkflowId,
		ArchetypeID: archetypeID,
		Tasks: map[tasks.Category][]tasks.Task{
			tasks.CategoryReplication: replicationTasks,
		},
	})
	if err != nil {
		return nil, err
	}

	historyLength := max(mutableState.GetNextEventID()-1, 0)
	return &historyservice.GenerateLastHistoryReplicationTasksResponse{
		StateTransitionCount: stateTransitionCount,
		HistoryLength:        historyLength,
	}, nil
}
