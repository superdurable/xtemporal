package replication

import (
	"context"

	"github.com/superdurable/temporal-server/api/historyservice/v1"
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/log/tag"
	"github.com/superdurable/temporal-server/service/history/replication"
)

func SyncWorkflowState(
	ctx context.Context,
	request *historyservice.SyncWorkflowStateRequest,
	replicationProgressCache replication.ProgressCache,
	syncStateRetriever replication.SyncStateRetriever,
	logger log.Logger,
) (_ *historyservice.SyncWorkflowStateResponse, retError error) {
	archetypeID := request.GetArchetypeId()
	if archetypeID == chasm.UnspecifiedArchetypeID {
		archetypeID = chasm.WorkflowArchetypeID
	}
	result, err := syncStateRetriever.GetSyncWorkflowStateArtifact(
		ctx,
		request.GetNamespaceId(),
		request.Execution,
		archetypeID,
		request.VersionedTransition,
		request.VersionHistories,
	)
	if err != nil {
		logger.Error("SyncWorkflowState failed to retrieve sync state artifact", tag.WorkflowNamespaceID(request.NamespaceId),
			tag.WorkflowID(request.Execution.WorkflowId),
			tag.WorkflowRunID(request.Execution.RunId),
			tag.Error(err))
		return nil, err
	}

	err = replicationProgressCache.Update(request.Execution.RunId, request.TargetClusterId, result.VersionedTransitionHistory, result.SyncedVersionHistory.Items)
	if err != nil {
		logger.Error("SyncWorkflowState failed to update progress cache",
			tag.WorkflowNamespaceID(request.NamespaceId),
			tag.WorkflowID(request.Execution.WorkflowId),
			tag.WorkflowRunID(request.Execution.RunId),
			tag.Error(err))
	}

	return &historyservice.SyncWorkflowStateResponse{
		VersionedTransitionArtifact: result.VersionedTransitionArtifact,
	}, nil
}
