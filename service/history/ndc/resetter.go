package ndc

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/common"
	"github.com/superdurable/temporal-server/common/definition"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/common/persistence"
	"github.com/superdurable/temporal-server/common/persistence/versionhistory"
	serviceerrors "github.com/superdurable/temporal-server/common/serviceerror"
	"github.com/superdurable/temporal-server/common/util"
	historyi "github.com/superdurable/temporal-server/service/history/interfaces"
	"go.temporal.io/api/serviceerror"
)

const (
	resendOnResetWorkflowMessage = "Resend events due to reset workflow"
)

type (
	resetter interface {
		resetWorkflow(
			ctx context.Context,
			now time.Time,
			baseLastEventID int64,
			baseLastEventVersion int64,
			incomingFirstEventID int64,
			incomingFirstEventVersion int64,
		) (historyi.MutableState, error)
	}

	resetterImpl struct {
		shard          historyi.ShardContext
		transactionMgr TransactionManager
		executionMgr   persistence.ExecutionManager
		stateRebuilder StateRebuilder

		namespaceID namespace.ID
		workflowID  string
		baseRunID   string
		newContext  historyi.WorkflowContext
		newRunID    string

		logger log.Logger
	}
)

var _ resetter = (*resetterImpl)(nil)

func NewResetter(
	shard historyi.ShardContext,
	transactionMgr TransactionManager,
	namespaceID namespace.ID,
	workflowID string,
	baseRunID string,
	newContext historyi.WorkflowContext,
	newRunID string,
	logger log.Logger,
) *resetterImpl {

	return &resetterImpl{
		shard:          shard,
		transactionMgr: transactionMgr,
		executionMgr:   shard.GetExecutionManager(),
		stateRebuilder: NewStateRebuilder(shard, logger),

		namespaceID: namespaceID,
		workflowID:  workflowID,
		baseRunID:   baseRunID,
		newContext:  newContext,
		newRunID:    newRunID,
		logger:      logger,
	}
}

func (r *resetterImpl) resetWorkflow(
	ctx context.Context,
	now time.Time,
	baseLastEventID int64,
	baseLastEventVersion int64,
	incomingFirstEventID int64,
	incomingFirstEventVersion int64,
) (historyi.MutableState, error) {

	baseBranchToken, err := r.getBaseBranchToken(
		ctx,
		baseLastEventID,
		baseLastEventVersion,
		incomingFirstEventID,
		incomingFirstEventVersion,
	)

	if err != nil {
		return nil, err
	}

	resetBranchToken, err := r.getResetBranchToken(ctx, baseBranchToken, baseLastEventID)
	if err != nil {
		return nil, err
	}

	requestID := uuid.NewString()
	rebuildMutableState, rebuildStats, err := r.stateRebuilder.Rebuild(
		ctx,
		now,
		definition.NewWorkflowKey(
			r.namespaceID.String(),
			r.workflowID,
			r.baseRunID,
		),
		baseBranchToken,
		baseLastEventID,
		util.Ptr(baseLastEventVersion),
		definition.NewWorkflowKey(
			r.namespaceID.String(),
			r.workflowID,
			r.newRunID,
		),
		resetBranchToken,
		requestID,
	)
	if err != nil {
		return nil, err
	}
	rebuildMutableState.AddHistorySize(rebuildStats.HistorySize)
	rebuildMutableState.AddExternalPayloadSize(rebuildStats.ExternalPayloadSize)
	rebuildMutableState.AddExternalPayloadCount(rebuildStats.ExternalPayloadCount)

	if err := rebuildMutableState.RefreshExpirationTimeoutTask(ctx); err != nil {
		return nil, err
	}

	r.newContext.Clear()
	return rebuildMutableState, nil
}

func (r *resetterImpl) getBaseBranchToken(
	ctx context.Context,
	baseLastEventID int64,
	baseLastEventVersion int64,
	incomingFirstEventID int64,
	incomingFirstEventVersion int64,
) (baseBranchToken []byte, retError error) {

	baseWorkflow, err := r.transactionMgr.LoadWorkflow(
		ctx,
		r.namespaceID,
		r.workflowID,
		r.baseRunID,
		chasm.WorkflowArchetypeID,
	)
	switch err.(type) {
	case nil:
		defer func() {
			baseWorkflow.GetReleaseFn()(retError)
		}()

		baseVersionHistories := baseWorkflow.GetMutableState().GetExecutionInfo().GetVersionHistories()
		index, err := versionhistory.FindFirstVersionHistoryIndexByVersionHistoryItem(
			baseVersionHistories,
			versionhistory.NewVersionHistoryItem(baseLastEventID, baseLastEventVersion),
		)
		if err != nil {
			// the base event and incoming event are from different branch
			// only re-replicate the gap on the incoming branch
			// the base branch event will eventually arrived
			return nil, serviceerrors.NewRetryReplication(
				resendOnResetWorkflowMessage,
				r.namespaceID.String(),
				r.workflowID,
				r.newRunID,
				common.EmptyEventID,
				common.EmptyVersion,
				incomingFirstEventID,
				incomingFirstEventVersion,
			)
		}

		baseVersionHistory, err := versionhistory.GetVersionHistory(baseVersionHistories, index)
		if err != nil {
			return nil, err
		}
		return baseVersionHistory.GetBranchToken(), nil
	case *serviceerror.NotFound:
		return nil, serviceerrors.NewRetryReplication(
			resendOnResetWorkflowMessage,
			r.namespaceID.String(),
			r.workflowID,
			r.newRunID,
			common.EmptyEventID,
			common.EmptyVersion,
			incomingFirstEventID,
			incomingFirstEventVersion,
		)
	default:
		return nil, err
	}
}

func (r *resetterImpl) getResetBranchToken(
	ctx context.Context,
	baseBranchToken []byte,
	baseLastEventID int64,
) ([]byte, error) {

	// fork a new history branch
	shardID := r.shard.GetShardID()
	resp, err := r.executionMgr.ForkHistoryBranch(ctx, &persistence.ForkHistoryBranchRequest{
		ForkBranchToken: baseBranchToken,
		ForkNodeID:      baseLastEventID + 1,
		Info:            persistence.BuildHistoryGarbageCleanupInfo(r.namespaceID.String(), r.workflowID, r.newRunID),
		ShardID:         shardID,
		NamespaceID:     r.namespaceID.String(),
		NewRunID:        r.newRunID,
	})
	if err != nil {
		return nil, err
	}

	return resp.NewBranchToken, nil
}
