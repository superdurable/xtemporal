package signalwithstartworkflow

import (
	"context"

	"github.com/superdurable/xtemporal/api/historyservice/v1"
	"github.com/superdurable/xtemporal/api/matchingservice/v1"
	"github.com/superdurable/xtemporal/common/definition"
	"github.com/superdurable/xtemporal/common/locks"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/worker_versioning"
	"github.com/superdurable/xtemporal/service/history/api"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
)

func Invoke(
	ctx context.Context,
	signalWithStartRequest *historyservice.SignalWithStartWorkflowExecutionRequest,
	shard historyi.ShardContext,
	workflowConsistencyChecker api.WorkflowConsistencyChecker,
	matchingClient matchingservice.MatchingServiceClient,
	versionCache worker_versioning.VersionMembershipAndReactivationStatusCache,
	reactivationSignaler api.VersionReactivationSignalerFn,
) (_ *historyservice.SignalWithStartWorkflowExecutionResponse, retError error) {
	namespaceEntry, err := api.GetActiveNamespace(shard, namespace.ID(signalWithStartRequest.GetNamespaceId()), signalWithStartRequest.SignalWithStartRequest.WorkflowId)
	if err != nil {
		return nil, err
	}
	namespaceID := namespaceEntry.ID()

	var currentWorkflowLease api.WorkflowLease
	currentWorkflowLease, err = workflowConsistencyChecker.GetWorkflowLease(
		ctx,
		nil,
		definition.NewWorkflowKey(
			string(namespaceID),
			signalWithStartRequest.SignalWithStartRequest.WorkflowId,
			"",
		),
		locks.PriorityHigh,
	)
	switch err.(type) {
	case nil:
		defer func() { currentWorkflowLease.GetReleaseFn()(retError) }()
	case *serviceerror.NotFound:
		currentWorkflowLease = nil
	default:
		return nil, err
	}

	startRequest := ConvertToStartRequest(
		namespaceID,
		signalWithStartRequest.SignalWithStartRequest,
		shard.GetTimeSource().Now(),
	)
	request := startRequest.StartRequest

	api.MigrateWorkflowIDReusePolicyForRunningWorkflow(
		&signalWithStartRequest.SignalWithStartRequest.WorkflowIdReusePolicy,
		&signalWithStartRequest.SignalWithStartRequest.WorkflowIdConflictPolicy)

	api.OverrideStartWorkflowExecutionRequest(request, metrics.HistorySignalWithStartWorkflowExecutionScope, shard, shard.GetMetricsHandler())

	err = api.ValidateStartWorkflowExecutionRequest(ctx, request, shard, namespaceEntry, "SignalWithStartWorkflowExecution")
	if err != nil {
		return nil, err
	}

	// Validation for versioning override, if any.
	shouldSkipReactivation, revisionNumber, err := worker_versioning.ValidateVersioningOverrideAndGetReactivationEligibility(ctx, request.GetVersioningOverride(), matchingClient, versionCache, request.GetTaskQueue().GetName(), enumspb.TASK_QUEUE_TYPE_WORKFLOW, namespaceID.String())
	if err != nil {
		return nil, err
	}

	outcome, err := SignalWithStartWorkflow(
		ctx,
		shard,
		namespaceEntry,
		currentWorkflowLease,
		startRequest,
		signalWithStartRequest.SignalWithStartRequest,
	)
	if err != nil {
		return nil, err
	}

	// Notify version workflow if we're starting a new workflow pinned to a potentially drained version
	if outcome.started {
		api.ReactivateVersionWorkflowIfPinned(ctx, namespaceEntry, request.GetVersioningOverride(), reactivationSignaler, shard.GetConfig().EnableVersionReactivationSignals(), shouldSkipReactivation, revisionNumber)
	}

	swr := signalWithStartRequest.SignalWithStartRequest
	return &historyservice.SignalWithStartWorkflowExecutionResponse{
		RunId:               outcome.runID,
		FirstExecutionRunId: outcome.firstExecutionRunID,
		Started:             outcome.started,
		SignalLink: api.GenerateRequestIDRefLink(
			swr.GetNamespace(),
			swr.GetWorkflowId(),
			outcome.runID,
			swr.GetRequestId(),
			enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED,
		),
	}, nil
}
